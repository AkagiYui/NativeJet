#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <Security/Security.h>
#include <Security/Authorization.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <libproc.h>
#include <bsm/audit.h>
#include <unistd.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

static int fail(char **error, const char *message) {
    if (error) *error = strdup(message);
    return -1;
}
int nj_peer_pid(int fd) {
    pid_t pid = 0;
    socklen_t size = sizeof(pid);
    if (getsockopt(fd, SOL_LOCAL, LOCAL_PEERPID, &pid, &size) != 0) return -1;
    return (int)pid;
}

int nj_peer(int fd, const char *requirement, unsigned int *uid, char **error) {
    uid_t peer; gid_t group;
    if (getpeereid(fd, &peer, &group) != 0) return fail(error, "无法读取本地连接身份");
    *uid = peer;
    if (!requirement || !requirement[0]) return 0;
    audit_token_t token;
    socklen_t size = sizeof(token);
    if (getsockopt(fd, SOL_LOCAL, LOCAL_PEERTOKEN, &token, &size) != 0 || size != sizeof(token))
        return fail(error, "无法读取连接的 audit token");
    CFDataRef audit = CFDataCreate(NULL, (const UInt8 *)&token, sizeof(token));
    const void *keys[] = {kSecGuestAttributeAudit};
    const void *values[] = {audit};
    CFDictionaryRef attrs = CFDictionaryCreate(NULL, keys, values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    SecCodeRef code = NULL;
    OSStatus result = SecCodeCopyGuestWithAttributes(NULL, attrs, kSecCSDefaultFlags, &code);
    CFRelease(attrs); CFRelease(audit);
    if (result != errSecSuccess) return fail(error, "无法验证对端进程的动态代码签名");
    CFStringRef text = CFStringCreateWithCString(NULL, requirement, kCFStringEncodingUTF8);
    SecRequirementRef req = NULL;
    result = SecRequirementCreateWithString(text, kSecCSDefaultFlags, &req);
    CFRelease(text);
    if (result == errSecSuccess) result = SecCodeCheckValidity(code, kSecCSStrictValidate, req);
    if (req) CFRelease(req);
    CFRelease(code);
    if (result != errSecSuccess) return fail(error, "对端程序的签名不符合 NativeJet 身份要求");
    return 0;
}

int nj_login_item(const char *action, char **error) {
    @autoreleasepool {
        if (@available(macOS 13.0, *)) {
            NSBundle *bundle = [NSBundle mainBundle];
            if (![bundle.bundleURL.pathExtension isEqualToString:@"app"] || !bundle.bundleIdentifier.length)
                return fail(error, "请从打包后的 NativeJet.app 启动，再设置登录项");
            SMAppService *service = [SMAppService mainAppService];
            SMAppServiceStatus status = service.status;
            NSError *problem = nil;
            if (!strcmp(action, "register")) {
                if (status != SMAppServiceStatusEnabled && ![service registerAndReturnError:&problem] &&
                    service.status != SMAppServiceStatusEnabled && service.status != SMAppServiceStatusRequiresApproval)
                    return fail(error, problem.localizedDescription.UTF8String ?: "无法添加登录项");
            } else if (!strcmp(action, "unregister")) {
                if (status != SMAppServiceStatusNotRegistered && status != SMAppServiceStatusNotFound &&
                    ![service unregisterAndReturnError:&problem] &&
                    service.status != SMAppServiceStatusNotRegistered)
                    return fail(error, problem.localizedDescription.UTF8String ?: "无法移除登录项");
            } else if (!strcmp(action, "settings")) {
                [SMAppService openSystemSettingsLoginItems];
            } else if (strcmp(action, "status")) {
                return fail(error, "未知登录项操作");
            }
            status = service.status;
            // macOS can report NotFound for a valid app that hasn't registered yet.
            return (int)(status == SMAppServiceStatusNotFound ? SMAppServiceStatusNotRegistered : status);
        }
        return fail(error, "需要 macOS 13 或更新版本");
    }
}

long long nj_process(int pid, char *path, int capacity) {
    struct proc_bsdinfo info;
    if (proc_pidpath(pid, path, (uint32_t)capacity) <= 0) return 0;
    if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info)) return 0;
    return (long long)info.pbi_start_tvsec * 1000000 + info.pbi_start_tvusec;
}

char *nj_requirement(const char *path, char **error) {
    CFURLRef url = CFURLCreateFromFileSystemRepresentation(NULL, (const UInt8 *)path, strlen(path), false);
    SecStaticCodeRef code = NULL;
    OSStatus result = SecStaticCodeCreateWithPath(url, kSecCSDefaultFlags, &code);
    CFRelease(url);
    if (result == errSecSuccess) result = SecStaticCodeCheckValidity(code, kSecCSStrictValidate, NULL);
    if (result != errSecSuccess) {
        if (code) CFRelease(code);
        fail(error, "无法验证指定程序的代码签名");
        return NULL;
    }
    SecRequirementRef req = NULL;
    result = SecCodeCopyDesignatedRequirement(code, kSecCSDefaultFlags, &req);
    CFRelease(code);
    CFStringRef text = NULL;
    if (result == errSecSuccess) result = SecRequirementCopyString(req, kSecCSDefaultFlags, &text);
    if (req) CFRelease(req);
    if (result != errSecSuccess) { fail(error, "无法读取程序签名要求"); return NULL; }
    CFIndex length = CFStringGetMaximumSizeForEncoding(CFStringGetLength(text), kCFStringEncodingUTF8) + 1;
    char *output = malloc((size_t)length);
    if (!CFStringGetCString(text, output, length, kCFStringEncodingUTF8)) {
        free(output); output = NULL; fail(error, "无法转换签名要求");
    }
    CFRelease(text);
    return output;
}

int nj_check_signature(const char *path, const char *requirement, int self, char **error) {
    CFStringRef text = CFStringCreateWithCString(NULL, requirement, kCFStringEncodingUTF8);
    SecRequirementRef req = NULL;
    OSStatus result = SecRequirementCreateWithString(text, kSecCSDefaultFlags, &req);
    CFRelease(text);
    if (result == errSecSuccess && self) {
        SecCodeRef code = NULL;
        result = SecCodeCopySelf(kSecCSDefaultFlags, &code);
        if (result == errSecSuccess) result = SecCodeCheckValidity(code, kSecCSStrictValidate, req);
        if (code) CFRelease(code);
    } else if (result == errSecSuccess) {
        CFURLRef url = CFURLCreateFromFileSystemRepresentation(NULL, (const UInt8 *)path, strlen(path), false);
        SecStaticCodeRef code = NULL;
        result = SecStaticCodeCreateWithPath(url, kSecCSDefaultFlags, &code);
        CFRelease(url);
        if (result == errSecSuccess) result = SecStaticCodeCheckValidity(code, kSecCSStrictValidate, req);
        if (code) CFRelease(code);
    }
    if (req) CFRelease(req);
    if (result != errSecSuccess) return fail(error, "程序签名与授权记录不一致");
    return 0;
}

// Local ad-hoc builds cannot enroll through the Developer ID service path.
// Authorization Services only launches Apple's /bin/sh. Its fixed script
// copies the installer into a root-private directory and checks its digest
// before executing any application-owned code as root.
int nj_authorized_script(const char *script, char **output, char **error) {
    AuthorizationRef auth = NULL;
    OSStatus result = AuthorizationCreate(NULL, kAuthorizationEmptyEnvironment, kAuthorizationFlagDefaults, &auth);
    AuthorizationItem item = {kAuthorizationRightExecute, 0, NULL, 0};
    AuthorizationRights rights = {1, &item};
    char prompt[] = "NativeJet 需要安装或更新后台服务，以运行已授权内核并管理 TUN。";
    AuthorizationItem environmentItem = {kAuthorizationEnvironmentPrompt, sizeof(prompt), prompt, 0};
    AuthorizationEnvironment environment = {1, &environmentItem};
    if (result == errAuthorizationSuccess)
        result = AuthorizationCopyRights(auth, &rights, &environment,
            kAuthorizationFlagInteractionAllowed | kAuthorizationFlagExtendRights | kAuthorizationFlagPreAuthorize, NULL);
    FILE *pipe = NULL;
    if (result == errAuthorizationSuccess) {
        char *args[] = {"-c", (char *)script, NULL};
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        result = AuthorizationExecuteWithPrivileges(auth, "/bin/sh", kAuthorizationFlagDefaults, args, &pipe);
#pragma clang diagnostic pop
    }
    if (result != errAuthorizationSuccess) {
        if (auth) AuthorizationFree(auth, kAuthorizationFlagDestroyRights);
        if (result == errAuthorizationCanceled) return fail(error, "管理员授权已取消，后台服务未更改");
        char message[160];
        snprintf(message, sizeof(message), "无法启动系统授权安装器，系统错误 %d", (int)result);
        return fail(error, message);
    }
    char *buffer = calloc(65537, 1);
    size_t used = 0;
    char part[4096];
    if (pipe) {
        size_t n;
        while ((n = fread(part, 1, sizeof(part), pipe)) > 0) {
            size_t left = 65536 - used;
            size_t take = n < left ? n : left;
            memcpy(buffer + used, part, take); used += take;
        }
        fclose(pipe);
    }
    AuthorizationFree(auth, kAuthorizationFlagDestroyRights);
    *output = buffer;
    return 0;
}
