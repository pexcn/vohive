package modem

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"go.bug.st/serial"
)

// SerialAT 是一个轻量级的串口包装器，用于在不实例化整个 Manager 的情况下执行简单的 AT 命令
type SerialAT struct {
	port serial.Port
}

// With a USSD request, OK acknowledges the command; +CUSD carries the later
// network reply. Keep the transient port open until both have arrived.
var serialATUSSDRequest = regexp.MustCompile(`(?i)^\s*AT\+CUSD\s*=\s*1\s*,`)

type serialATReply struct {
	line     strings.Builder
	waitUSSD bool
	ok       bool
	ussd     bool
	inUSSD   bool
	quoted   bool
}

func (r *serialATReply) consume(data []byte) bool {
	for _, b := range data {
		r.line.WriteByte(b)
		if b != '\n' {
			continue
		}
		line := strings.TrimSpace(r.line.String())
		r.line.Reset()
		if r.inUSSD || strings.HasPrefix(line, "+CUSD:") {
			// A quoted menu can span multiple lines, including lines saying OK
			// or ERROR. These are text, not terminal result codes.
			r.inUSSD = true
			for i := 0; i < len(line); i++ {
				if r.quoted && line[i] == '\\' && i+1 < len(line) {
					i++
				} else if line[i] == '"' {
					r.quoted = !r.quoted
				}
			}
			if !r.quoted {
				r.ussd = true
				r.inUSSD = false
			}
		} else if line == "ERROR" || strings.HasPrefix(line, "+CME ERROR:") || strings.HasPrefix(line, "+CMS ERROR:") {
			return true
		} else if line == "OK" {
			r.ok = true
		}
		if r.ok && (!r.waitUSSD || r.ussd) {
			return true
		}
	}
	return false
}

// NewSerialAT 打开串口并返回 SerialAT 实例
func NewSerialAT(portName string, baudRate int, dataBits int, stopBits int, parity string) (*SerialAT, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: dataBits,
	}

	switch stopBits {
	case 1:
		mode.StopBits = serial.OneStopBit
	case 2:
		mode.StopBits = serial.TwoStopBits
	default:
		mode.StopBits = serial.OneStopBit
	}

	switch strings.ToUpper(parity) {
	case "N":
		mode.Parity = serial.NoParity
	case "O":
		mode.Parity = serial.OddParity
	case "E":
		mode.Parity = serial.EvenParity
	default:
		mode.Parity = serial.NoParity
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, err
	}

	// 设置默认超时
	if err := port.SetReadTimeout(time.Second); err != nil {
		port.Close()
		return nil, err
	}

	return &SerialAT{port: port}, nil
}

// Close 关闭串口
func (s *SerialAT) Close() error {
	return s.port.Close()
}

// Execute 发送 AT 命令并等待响应
func (s *SerialAT) Execute(cmd string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	reply := serialATReply{waitUSSD: serialATUSSDRequest.MatchString(cmd)}

	// 临时调整超时
	if err := s.port.SetReadTimeout(100 * time.Millisecond); err != nil {
		return "", err
	}

	// 清空缓冲区
	buf := make([]byte, 1024)
	for {
		n, _ := s.port.Read(buf)
		if n == 0 {
			break
		}
	}

	if err := s.port.SetReadTimeout(200 * time.Millisecond); err != nil {
		return "", err
	}

	if !strings.HasSuffix(cmd, "\r\n") {
		cmd += "\r\n"
	}

	if _, err := s.port.Write([]byte(cmd)); err != nil {
		return "", err
	}

	deadline := time.Now().Add(timeout)
	var response strings.Builder

	for time.Now().Before(deadline) {
		n, err := s.port.Read(buf)
		if n > 0 {
			response.Write(buf[:n])
			if reply.consume(buf[:n]) {
				return response.String(), nil
			}
		}
		if err != nil {
			// ignore read errors (timeouts)
			continue
		}
	}

	if reply.waitUSSD && reply.ok {
		return response.String(), errors.New("等待 USSD 网络回复超时（已收到 OK，未收到完整 +CUSD）")
	}
	return response.String(), errors.New("timeout")
}
