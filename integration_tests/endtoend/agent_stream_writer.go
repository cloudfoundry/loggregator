package endtoend

import (
	"fmt"
	"net"
)

type AgentStreamWriter struct {
	agentConn net.Conn
	Writes    int
}

func NewAgentStreamWriter(agentPort int) *AgentStreamWriter {
	agentConn, err := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", agentPort))
	if err != nil {
		panic(err)
	}
	return &AgentStreamWriter{agentConn: agentConn}
}

func (w *AgentStreamWriter) Write(b []byte) {
	w.Writes++
	w.agentConn.Write(b)
}
