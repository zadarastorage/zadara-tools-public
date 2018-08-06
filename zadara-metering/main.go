// Copyright © 2018 Zadara Storage
// Originally Authored By: Jeremy Brown <jeremy@zadarastorage.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// zsnaputil is a Go program that will extract and process zsnaps for metering
// and log information
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	jww "github.com/spf13/jwalterweatherman"
)

const version = "0.1"

var (
	showVersion   bool
	verbose       bool
	meteringPath  string
	meteringFiles []string
)

func usage() {
	// Output when illegal arguments are passed
	fmt.Printf("Usage: %v path/to/meteringfile.db\n", os.Args[0])
	fmt.Println()
	flag.PrintDefaults()
}

func init() {
	flag.BoolVar(&verbose, "v", false, "enable verbose output")
	flag.BoolVar(&showVersion, "V", false, "display version and exit")

	// Require a single positional argument - the zsnap or glob of zsnaps to be processed
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if len(flag.Args()) != 1 {
		fmt.Println("Usage: zadara-metering <path/to/metering/files>")
		os.Exit(1)
	}

	meteringPath = flag.Args()[0]

	if verbose {
		jww.SetStdoutThreshold(jww.LevelDebug)
	} else {
		jww.SetStdoutThreshold(jww.LevelInfo)
	}
}

func run() (err error) {
	var vpsa string

	jww.INFO.Printf("processing new metering files provided at: %v", meteringPath)
	meteringFiles, err = findMeteringFiles(meteringPath)
	if err != nil {
		return err
	}

	for _, file := range meteringFiles {
		vpsa, err = getVPSANameFromPath(file)

		if err != nil {
			return err
		}

		if err = processMeteringFiles(file, vpsa); err != nil {
			return err
		}

		if err = os.Remove(file); err != nil {
			jww.WARN.Println(err)
		}

		empty, err := isDirEmpty(filepath.Dir(file))

		if err != nil {
			return err
		}

		if empty {
			if err = os.Remove(filepath.Dir(file)); err != nil {
				jww.WARN.Println(err)
			}
		}
	}

	fmt.Println("Processing of metering databases complete...")

	return nil
}

func main() {
	if err := run(); err != nil {
		jww.ERROR.Println(err)
		os.Exit(1)
	}
}
