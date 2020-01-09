// Copyright 2015 Matthew Holt and The Caddy Authors
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

// Package main is the entry point of the Caddy application.
// Most of Caddy's functionality is provided through modules,
// which can be plugged in by adding their import below.
//
// There is no need to modify the Caddy source code to customize your
// builds. You can easily build a custom Caddy with these simple steps:
//
//   1. Copy this file (main.go) into a new folder
//   2. Edit the imports below to include the modules you want plugged in
//   3. Run `go mod init caddy`
//   4. Run `go install` or `go build` - you now have a custom binary!
//
package main

import (
	// _ "net/http/pprof"
	// "net/http"
	// "os"
	// plug in Caddy modules here

	// "github.com/google/logger"

	"github.com/caddyserver/caddy/v2"
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

const (
	logPath = "./output.log"
	verbose = true
  )

func main() {
	// logFile, err := os.OpenFile(logPath, os.O_CREATE | os.O_WRONLY | os.O_APPEND, 0600)
	// if err != nil {
	//   logger.Fatalf("error in initiatlizing global logger, err: <%s>\n", err.Error())
	// }
	// defer logFile.Close()
  
	// defer logger.Init("Caddy Logger", verbose, true, logFile).Close()
  
	// logger.Info("logger init")
  
	// go func() {
	//   logger.Info(http.ListenAndServe("0.0.0.0:6060", nil))
	// }()

	// 在后台运行调度器
	go caddy.RunResponseWriterScheduler()
	caddycmd.Main()
}
