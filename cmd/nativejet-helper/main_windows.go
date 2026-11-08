package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"

	"nativejet/internal/buildinfo"
	"nativejet/internal/installation"
	"nativejet/internal/kernel"
	"nativejet/internal/platform"
	"nativejet/internal/service"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--interrupt-core" {
		if err := platform.SendCoreInterrupt(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	development := flag.Bool("development", false, "run a per-user non-TUN helper")
	socket := flag.String("socket", buildinfo.Socket, "local named pipe")
	data := flag.String("data-dir", "", "development state directory")
	inbox := flag.String("kernel-dir", "", "development kernel directory")
	install := flag.Bool("install-service", false, "install/update the Windows service")
	remove := flag.Bool("remove-service", false, "remove the Windows service")
	payload := flag.String("payload", "", "installation payload directory")
	digest := flag.String("digest", "", "installation payload SHA-256")
	installed := flag.Bool("installed", false, "run under Windows SCM")
	flag.Parse()
	if *install {
		if err := installation.Install(*payload, *digest, *remove); err != nil {
			reportFailure(err)
			log.Fatal(err)
		}
		return
	}
	if *installed && !*development && *data == "" && *inbox == "" && *socket == buildinfo.Socket {
		if err := svc.Run(buildinfo.HelperID, installedService{}); err != nil {
			reportFailure(err)
			log.Fatal(err)
		}
		return
	}
	if !*development || *installed || !filepath.IsAbs(*data) || !filepath.IsAbs(*inbox) || *socket == buildinfo.Socket {
		log.Fatal("development requires a separate named pipe and absolute data/kernel directories")
	}
	if err := platform.SuperviseChildren(); err != nil {
		log.Fatal(err)
	}
	sid, err := platform.CurrentSID()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	server := service.Server{Development: true, Socket: *socket, Data: *data, Inbox: *inbox, OwnerSID: sid}
	if err = server.Serve(ctx); err != nil {
		log.Fatal(err)
	}
}

type installedService struct{}

func (installedService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending, WaitHint: 10000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- runInstalled(ctx, ready) }()
	current := svc.Status{State: svc.StartPending, WaitHint: 10000}
	var checkpoint uint32
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				reportFailure(err)
				return true, 1
			}
			return false, 0
		case <-ready:
			ready = nil
			if current.State != svc.StopPending {
				current = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
				status <- current
			}
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				current = svc.Status{State: svc.StopPending, WaitHint: 12000}
				status <- current
				cancel()
			}
		case <-ticker.C:
			if current.State == svc.StartPending || current.State == svc.StopPending {
				checkpoint++
				current.CheckPoint = checkpoint
				status <- current
			}
		}
	}
}

func runInstalled(ctx context.Context, ready chan struct{}) error {
	sid, err := platform.CurrentSID()
	if err != nil || sid != "S-1-5-18" {
		return errors.New("installed helper requires LocalSystem under SCM")
	}
	r, err := installation.LoadRegistration()
	if err != nil {
		return err
	}
	if err = r.VerifyServiceDirectories(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	hash, err := kernel.Hash(exe)
	if err != nil || hash != r.HelperSHA256 || !platform.SamePath(exe, r.HelperPath()) {
		return errors.New("installed helper identity changed")
	}
	file, err := os.OpenFile(filepath.Join(r.Data(), "helper.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = platform.SuperviseChildren(); err != nil {
		_, _ = fmt.Fprintln(file, err)
		return err
	}
	approved := make(map[string]bool, len(r.ApprovedKernels))
	for _, id := range r.ApprovedKernels {
		approved[id] = true
	}
	server := service.Server{Installed: true, Socket: buildinfo.Socket, Data: r.Data(), Inbox: r.Inbox(),
		OwnerSID: r.OwnerSID, ApprovedKernels: approved, Ready: ready}
	err = server.Serve(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(file, err)
	}
	return err
}

func reportFailure(err error) {
	event, openErr := eventlog.Open("Application")
	if openErr == nil {
		defer event.Close()
		_ = event.Error(1, "NativeJet: "+err.Error())
	}
}
