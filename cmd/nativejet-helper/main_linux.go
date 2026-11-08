package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"nativejet/internal/buildinfo"
	"nativejet/internal/installation"
	"nativejet/internal/service"
)

func main() {
	development := flag.Bool("development", false, "run the helper as the logged-in user")
	socket := flag.String("socket", buildinfo.Socket, "local IPC socket")
	data := flag.String("data-dir", "", "development state directory")
	inbox := flag.String("kernel-dir", "", "development kernel directory")
	install := flag.Bool("install-service", false, "apply an administrator-authorized service change")
	remove := flag.Bool("remove-service", false, "disable the service while retaining its data")
	payload := flag.String("payload", "", "private installation payload directory")
	digest := flag.String("digest", "", "installation payload SHA-256")
	installed := flag.Bool("installed", false, "run the installed systemd service")
	flag.Parse()

	syscall.Umask(0077)
	if *install {
		if err := installation.Install(*payload, *digest, *remove); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if *installed {
		if *development || *data != "" || *inbox != "" || *socket != buildinfo.Socket {
			log.Fatal("systemd helper paths are fixed")
		}
		if err := runInstalled(ctx, stop); err != nil {
			log.Fatal(err)
		}
		return
	}
	if !*development || os.Geteuid() == 0 || *data == "" || *inbox == "" {
		log.Fatal("run with --development as a logged-in user and specify socket, data and kernel directories")
	}
	server := service.Server{Development: true, Socket: *socket, Data: *data, Inbox: *inbox,
		OwnerUID: os.Getuid(), OwnerGID: os.Getgid()}
	if err := server.Serve(ctx); err != nil {
		log.Fatal(err)
	}
}

func runInstalled(ctx context.Context, stop context.CancelFunc) error {
	notifySocket := os.Getenv("NOTIFY_SOCKET")
	exe, err := os.Executable()
	if os.Geteuid() != 0 || err != nil || exe != installation.HelperPath || notifySocket == "" {
		return errors.New("installed helper must run from the systemd unit")
	}
	r, err := installation.LoadRegistration()
	if err != nil {
		return err
	}
	if err = r.VerifyServiceDirectories(); err != nil {
		return err
	}
	approved := make(map[string]bool, len(r.ApprovedKernels))
	for _, id := range r.ApprovedKernels {
		approved[id] = true
	}
	ready := make(chan struct{})
	server := service.Server{Installed: true, Socket: buildinfo.Socket, Data: r.Data(), Inbox: r.Inbox(),
		OwnerUID: r.OwnerUID, OwnerGID: r.OwnerGID, ApprovedKernels: approved, Ready: ready}
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-ready:
		}
		if err := notifyReady(notifySocket); err != nil {
			log.Printf("systemd readiness notification failed: %v", err)
			stop()
		}
	}()
	return server.Serve(ctx)
}

func notifyReady(path string) error {
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte("READY=1"))
	return err
}
