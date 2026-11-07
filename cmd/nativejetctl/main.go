// nativejetctl is a client for the ordinary-user development helper.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"nativejet/internal/ipc"
)

func main() {
	socket := flag.String("socket", "", "development helper socket")
	payload := flag.String("params", "{}", "JSON parameters")
	flag.Parse()
	if flag.NArg() != 1 || *socket == "" {
		fmt.Fprintln(os.Stderr, "usage: nativejetctl -socket PATH [-params JSON] METHOD")
		os.Exit(2)
	}
	var params any
	if err := json.Unmarshal([]byte(*payload), &params); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var result any
	client := ipc.Client{Socket: *socket, Development: true}
	if err := client.Call(ctx, flag.Arg(0), params, &result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}
