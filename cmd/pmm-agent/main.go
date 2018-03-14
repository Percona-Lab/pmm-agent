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
	"context"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Percona-Lab/pmm-api/agent"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/Percona-Lab/pmm-agent/tunnel"
)

const (
	// FIXME remove
	uuid = "baf4e293-9f1c-4b3f-9244-02c8f3f37d9d"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	log.SetFlags(0)
	log.SetPrefix("stdlog: ")
	logrus.SetLevel(logrus.DebugLevel)
	kingpin.Parse()

	l := logrus.WithField("component", "main")
	defer l.Info("Done.")
	ctx, cancel := context.WithCancel(context.Background())

	// handle termination signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-signals
		signal.Stop(signals)
		l.Warnf("Got %v (%d) signal, shutting down...", s, s)
		cancel()
	}()

	// Connect to pmm-gateway. Once gRPC connection is established, client reconnects automatically.
	const addr = "127.0.0.1:8080"
	l.Infof("Connecting to pmm-gateway %s...", addr)
	var conn *grpc.ClientConn
	for conn == nil {
		// TODO configure backoff
		// TODO configure TLS
		var err error
		conn, err = grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			// l.Error(err)
		}
	}
	defer conn.Close()
	l.Infof("Connected to pmm-gateway (%s).", conn.GetState())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		agentCtx := metadata.AppendToOutgoingContext(ctx, "pmm-agent-uuid", uuid)
		tunnel.NewService(agent.NewTunnelsClient(conn)).Run(agentCtx)
	}()

	wg.Wait()
}
