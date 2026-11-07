package main

import (
	"flag"
	"log"

	"nativejet/internal/app"
)

func main() {
	dev := flag.Bool("dev", false, "force an unprivileged development helper")
	data := flag.String("data-dir", "", "application data directory")
	helper := flag.String("helper", "", "development helper executable")
	privilegedDev := flag.Bool("privileged-dev", false, "connect to a manually authorized, signature-pinned development helper")
	flag.Parse()
	a, err := app.New(app.Options{Development: !*privilegedDev, PrivilegedDevelopment: *privilegedDev, Data: *data, Helper: *helper, PreferInstalled: !*dev && *data == "" && !*privilegedDev})
	if err != nil {
		log.Fatal(err)
	}
	if err = a.Run(); err != nil {
		log.Fatal(err)
	}
}
