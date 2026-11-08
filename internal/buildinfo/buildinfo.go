package buildinfo

const AppID = "dev.nativejet.app"
const HelperID = "dev.nativejet.helper"
const ServicePlist = HelperID + ".plist"

var Version = "0.2.7"

// Optional Developer ID identity for the legacy service path. The GUI installer
// also supports ad-hoc builds by pinning code requirements and owner UID.
var TeamID string
