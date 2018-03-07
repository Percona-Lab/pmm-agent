// pmm-agent
// Copyright (C) 2018 Percona LLC
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"math/rand"
	"time"

	"github.com/Percona-Lab/wsrpc"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/Percona-Lab/pmm-agent/tunnel"
	"github.com/Percona-Lab/pmm-api/agent"
	"github.com/Percona-Lab/pmm-api/gateway"
)

func handleConn(conn *wsrpc.Conn) {
	logrus.Info("Connected!")
	defer conn.Close()

	server := tunnel.NewService(gateway.NewServiceClient(conn))
	err := agent.NewServiceDispatcher(conn, server).Run()
	logrus.Infof("Server exited with %v", err)
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	kingpin.Parse()

	const addr = "ws://127.0.0.1:7781/"
	logrus.Infof("Connecting to %s...", addr)
	for {
		conn, err := wsrpc.Dial(addr)
		if err != nil {
			logrus.Error(err)
			delay := time.Duration(rand.Float64()*2.0*float64(time.Second)) + time.Second
			time.Sleep(delay)
			continue
		}

		handleConn(conn)
	}
}
