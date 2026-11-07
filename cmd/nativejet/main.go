package main

import (
	"flag"
	"log"

	"github.com/egoist/mygo"
	"nativejet/internal/app"
)

func main() {
	dev := flag.Bool("dev", false, "force an unprivileged development helper")
	data := flag.String("data-dir", "", "application data directory")
	helper := flag.String("helper", "", "development helper executable")
	privilegedDev := flag.Bool("privileged-dev", false, "connect to a manually authorized, signature-pinned development helper")
	flag.Parse()
	mygo.App.SetName("NativeJet")
	a, err := app.New(app.Options{Development: !*privilegedDev, PrivilegedDevelopment: *privilegedDev, Data: *data, Helper: *helper, PreferInstalled: !*dev && *data == "" && !*privilegedDev})
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.WhenReady(a.Start)
	mygo.App.OnOpenURL(func(raw string) {
		// macOS can deliver the link before the main window is created.
		mygo.App.WhenReady(func() { a.OpenURL(raw) })
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
