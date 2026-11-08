// nativejetctl is a command-line client for the NativeJet helper.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"nativejet/internal/buildinfo"
	"nativejet/internal/installation"
	"nativejet/internal/ipc"
)

func main() {
	socket := flag.String("socket", "", "development helper socket")
	payload := flag.String("params", "{}", "JSON parameters")
	paramsFile := flag.String("params-file", "", "read JSON parameters from a file")
	installed := flag.Bool("installed", false, "connect to the installed system helper")
	flag.Parse()
	if flag.NArg() != 1 || (*socket == "" && !*installed) {
		fmt.Fprintln(os.Stderr, "usage: nativejetctl (-socket PATH | -installed) [-params JSON | -params-file PATH] METHOD")
		os.Exit(2)
	}
	if *socket == "" {
		*socket = buildinfo.Socket
	}
	data := []byte(*payload)
	if *paramsFile != "" {
		file, err := os.Open(*paramsFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		data, err = io.ReadAll(io.LimitReader(file, 12<<20+1))
		_ = file.Close()
		if err != nil || len(data) > 12<<20 {
			fmt.Fprintln(os.Stderr, "无法读取参数文件或文件超限")
			os.Exit(2)
		}
	}
	var params any
	if err := json.Unmarshal(data, &params); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var result any
	client := ipc.Client{Socket: *socket, Development: !*installed}
	if *installed {
		registration, err := installation.LoadRegistration()
		if err == nil {
			err = registration.VerifyOwner()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		client.Requirement = registration.HelperRequirement
	}
	if err := client.Call(ctx, flag.Arg(0), params, &result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ = json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}
