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

package tunnel

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/Percona-Lab/pmm-api/agent"
	"github.com/sirupsen/logrus"
)

type Service struct {
	client agent.TunnelsClient
	l      *logrus.Entry
}

func NewService(client agent.TunnelsClient) *Service {
	return &Service{
		client: client,
		l:      logrus.WithField("component", "tunnel"),
	}
}

func (s *Service) runTunnel(ctx context.Context, stream agent.Tunnels_MakeClient, dial string) {
	defer func() {
		stream.CloseSend()

		// drain stream
		var recvErr error
		for recvErr == nil {
			_, recvErr = stream.Recv()
		}
	}()

	// try to dial, send dial response in any case
	l := s.l.WithField("dial", dial)
	l.Info("Dialing...")
	c, dialErr := net.Dial("tcp", dial)
	var dialErrS string
	if dialErr != nil {
		dialErrS = dialErr.Error()
	}
	env := &agent.TunnelsEnvelopeFromAgent{
		Payload: &agent.TunnelsEnvelopeFromAgent_DialResponse{
			DialResponse: &agent.TunnelsDialResponse{
				Error: dialErrS,
			},
		},
	}
	if err := stream.Send(env); err != nil {
		l.Errorf("Failed to send message: %s", err)
		return
	}
	if dialErr != nil {
		l.Errorf("Failed to dial: %s", dialErr)
		return
	}

	conn := c.(*net.TCPConn)
	var wg sync.WaitGroup

	// receive messages until error, write to TCP connection
	wg.Add(1)
	go func() {
		defer func() {
			conn.CloseWrite()
			wg.Done()
		}()

		for {
			env, recvErr := stream.Recv()
			if recvErr != nil {
				l.Errorf("Failed to receive message: %s.", recvErr)
				return
			}
			data := env.GetData()
			if data == nil {
				l.Errorf("Expected data, got %s.", env)
				return
			}

			if len(data.Data) != 0 {
				l.Debugf("Writing %d bytes...", len(data.Data))
				if _, writeErr := conn.Write(data.Data); writeErr != nil {
					l.Errorf("Failed to write: %s.", writeErr)
					return
				}
			}
			if data.Error != "" {
				l.Errorf("Got error, exiting: %s.", data.Error)
				return
			}
		}
	}()

	// read from TCP connection until error, send messages
	wg.Add(1)
	go func() {
		defer func() {
			stream.CloseSend()
			conn.CloseRead()
			wg.Done()
		}()

		for {
			b := make([]byte, 4096)
			n, readErr := conn.Read(b)
			l.Debugf("Read %d bytes.", n)
			var readErrS string
			if readErr != nil {
				readErrS = readErr.Error()
			}
			env := &agent.TunnelsEnvelopeFromAgent{
				Payload: &agent.TunnelsEnvelopeFromAgent_Data{
					Data: &agent.TunnelsData{
						Error: readErrS,
						Data:  b[:n],
					},
				},
			}
			if err := stream.Send(env); err != nil {
				l.Errorf("Failed to send message: %s.", err)
				return
			}
			if readErr != nil {
				if readErr == io.EOF {
					l.Info("Read closed.")
				} else {
					l.Errorf("Failed to read: %s.", readErr)
				}
				return
			}
		}
	}()

	wg.Wait()
}

func (s *Service) Run(ctx context.Context) {
	defer s.l.Info("Done.")

	for {
		// make new stream
		var stream agent.Tunnels_MakeClient
		for stream == nil {
			if ctx.Err() != nil {
				s.l.Error(ctx.Err())
				return
			}

			// TODO configure backoff
			var err error
			stream, err = s.client.Make(ctx)
			if err != nil {
				s.l.Errorf("Failed to make new stream: %s.", err)
			}
		}
		s.l.Debug("New stream created.")

		// wait for dial request, start tunnel
		env, err := stream.Recv()
		if err != nil {
			s.l.Errorf("Failed to receive message: %s.", err)
			stream.CloseSend()
			continue
		}
		req := env.GetDialRequest()
		if req == nil {
			s.l.Errorf("Expected dial request, got %s.", env)
			stream.CloseSend()
			continue
		}
		s.l.Debugf("Got dial request, starting tunnel to %s.", req.Dial)
		go s.runTunnel(ctx, stream, req.Dial)
	}
}
