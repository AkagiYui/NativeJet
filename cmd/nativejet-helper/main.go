package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"nativejet/internal/buildinfo"
	"nativejet/internal/installation"
	"nativejet/internal/kernel"
	"nativejet/internal/platform"
	"nativejet/internal/service"
)

func main() {
	development := flag.Bool("development", false, "run an unprivileged, per-user development helper")
	socket := flag.String("socket", buildinfo.Socket, "local IPC socket")
	data := flag.String("data-dir", "", "development state directory")
	inbox := flag.String("kernel-dir", "", "development kernel directory")
	authorizeApp := flag.String("authorize-app", "", "root-only foreground development: pin the signature of this App")
	installService := flag.Bool("install-service", false, "run the administrator-authorized service installer")
	removeService := flag.Bool("remove-service", false, "remove the service while retaining user data")
	payload := flag.String("payload", "", "private installation payload")
	digest := flag.String("digest", "", "expected installation payload digest")
	installed := flag.Bool("installed", false, "run the installed local service")
	flag.Parse()
	if *installService {
		err := installation.Install(*payload, *digest, *removeService)
		result := installation.Result{}
		if err != nil {
			result.Error = err.Error()
		}
		_ = json.NewEncoder(os.Stdout).Encode(result)
		if err != nil {
			os.Exit(1)
		}
		return
	}
	if *development && os.Geteuid() == 0 {
		log.Fatal("development helper must run as the logged-in user")
	}
	if !*development && (*data != "" || *inbox != "" || *socket != buildinfo.Socket) {
		log.Fatal("production service paths are fixed")
	}
	requirement := ""
	var approved map[string]bool
	if *installed {
		if *development || *authorizeApp != "" || os.Geteuid() != 0 {
			log.Fatal("installed service requires root and fixed registration")
		}
		registration, err := installation.LoadRegistration()
		if err != nil {
			log.Fatal(err)
		}
		exe, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		hash, err := kernel.Hash(exe)
		if err != nil || hash != registration.HelperSHA256 || exe != installation.HelperPath {
			log.Fatal("installed helper identity changed")
		}
		approved = map[string]bool{}
		for _, id := range registration.ApprovedKernels {
			approved[id] = true
		}
	}
	if *authorizeApp != "" {
		if *development || os.Geteuid() != 0 || !filepath.IsAbs(*authorizeApp) {
			log.Fatal("--authorize-app requires root and an absolute signed App path")
		}
		var err error
		requirement, err = platform.SigningRequirement(*authorizeApp)
		if err != nil {
			log.Fatal(err)
		}
		log.Print("foreground privileged development helper: client signature pinned; exits on Ctrl+C")
	}
	if *development && (*data == "" || *inbox == "") {
		log.Fatal("development mode requires explicit data and kernel directories")
	}
	syscall.Umask(0077)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	server := &service.Server{Development: *development, Installed: *installed, Socket: *socket, Data: *data, Inbox: *inbox, TeamID: buildinfo.TeamID, PeerRequirement: requirement, ApprovedKernels: approved}
	if err := server.Serve(ctx); err != nil {
		log.Fatal(err)
	}
}
