// notices collects the original licenses of pinned Go dependencies.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	dest := flag.String("output", "licenses/dependencies", "license output directory")
	targetOS := flag.String("goos", "", "target operating system for dependency discovery")
	targetArch := flag.String("goarch", "", "target architecture for dependency discovery")
	flag.Parse()
	command := exec.Command("go", "list", "-deps", "-json", "./cmd/nativejet", "./cmd/nativejet-helper")
	command.Stderr = os.Stderr
	if *targetOS != "" && *targetArch != "" {
		command.Env = append(os.Environ(), "GOOS="+*targetOS, "GOARCH="+*targetArch, "CGO_ENABLED=0")
	}
	data, err := command.Output()
	if err != nil {
		log.Fatal(err)
	}
	if err = os.MkdirAll(*dest, 0755); err != nil {
		log.Fatal(err)
	}
	var index strings.Builder
	decoder := json.NewDecoder(bytes.NewReader(data))
	seen := map[string]bool{}
	for {
		var pkg struct {
			Module *struct {
				Path, Version, Dir string
				Main               bool
			}
		}
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		if pkg.Module == nil {
			continue
		}
		module := pkg.Module
		if module.Main || module.Dir == "" || seen[module.Path] {
			continue
		}
		seen[module.Path] = true
		base := filepath.Join(*dest, strings.ReplaceAll(module.Path, "/", "_")+"@"+module.Version)
		count := 0
		err = filepath.WalkDir(module.Dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			name := strings.ToUpper(entry.Name())
			if !strings.HasPrefix(name, "LICENSE") && !strings.HasPrefix(name, "COPYING") && !strings.HasPrefix(name, "NOTICE") {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				return nil
			}
			relative, _ := filepath.Rel(module.Dir, path)
			target := filepath.Join(base, relative)
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err = os.WriteFile(target, data, 0644); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
		if count == 0 {
			log.Fatalf("no license found for %s", module.Path)
		}
		fmt.Fprintf(&index, "%s %s\n", module.Path, module.Version)
	}
	data, err = os.ReadFile(filepath.Join(runtime.GOROOT(), "LICENSE"))
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(*dest, "GO-LICENSE"), data, 0644); err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(*dest, "INDEX.txt"), []byte(index.String()), 0644); err != nil {
		log.Fatal(err)
	}
}
