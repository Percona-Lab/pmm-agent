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
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Percona-Lab/pmm-api/agent"
	"github.com/Percona-Lab/pmm-api/gateway"
)

type Service struct {
	client gateway.ServiceClient

	rw      sync.RWMutex
	tunnels map[string]net.Conn
}

func NewService(client gateway.ServiceClient) *Service {
	return &Service{
		client:  client,
		tunnels: make(map[string]net.Conn),
	}
}

func (s *Service) CreateTunnel(req *agent.CreateTunnelRequest) (*agent.CreateTunnelResponse, error) {
	c, err := net.Dial("tcp", req.Dial)
	if err != nil {
		return &agent.CreateTunnelResponse{
			Error: err.Error(),
		}, nil
	}

	tunnelID := fmt.Sprintf("%s-%s-%d", c.LocalAddr().String(), c.RemoteAddr().String(), time.Now().UnixNano())
	s.rw.Lock()
	s.tunnels[tunnelID] = c
	s.rw.Unlock()

	go func() {
		defer c.Close()

		time.Sleep(time.Second) // FIXME HACK

		for {
			b := make([]byte, 4096)
			n, err := c.Read(b)
			if err != nil {
				logrus.Error(err)
				return
			}
			if n == 0 {
				continue
			}

			res, err := s.client.WriteToTunnel(&gateway.WriteToTunnelRequest{
				TunnelId: tunnelID,
				Data:     b[:n],
			})
			if err != nil {
				logrus.Error(err)
				return
			}
			if res.Error != "" {
				logrus.Error(res.Error)
				return
			}
		}
	}()

	return &agent.CreateTunnelResponse{TunnelId: tunnelID}, nil
}

func (s *Service) WriteToTunnel(req *agent.WriteToTunnelRequest) (*agent.WriteToTunnelResponse, error) {
	s.rw.RLock()
	c := s.tunnels[req.TunnelId]
	s.rw.RUnlock()
	if c == nil {
		return &agent.WriteToTunnelResponse{
			Error: fmt.Sprintf("no such tunnel: %s", req.TunnelId),
		}, nil
	}

	if _, err := c.Write(req.Data); err != nil {
		return &agent.WriteToTunnelResponse{
			Error: err.Error(),
		}, nil
	}
	return &agent.WriteToTunnelResponse{}, nil
}

// check interfaces
var _ agent.ServiceServer = (*Service)(nil)
