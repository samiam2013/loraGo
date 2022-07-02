package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/albenik/go-serial"
	"github.com/sirupsen/logrus"
)

func main() {
	// run a switch to find the LoRa Radio module connected to the FTDI chip based on
	//	which platform this code is running on
	var serialPath string
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("lsusb").Output()
		if err != nil {
			logrus.WithError(err).Fatal("Can't run lsusb command.")
		}
		fmt.Println("output: ", string(out))
		var ftdiID string
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "FT232R USB UART") {
				pattern := regexp.MustCompile(`Serial: (?P<SerialIdent>[a-zA-Z0-9]+)`)
				matches := pattern.FindStringSubmatch(line)
				identIdx := pattern.SubexpIndex("SerialIdent")
				ftdiID = matches[identIdx]
			}
		}
		serialPath = "/dev/cu.usbserial-" + ftdiID
		// check if there's a file availabe at the path
		if _, err := os.Stat(serialPath); err != nil {
			logrus.WithError(err).Fatal("Couldn't open serial path.")
		}
	default:
		logrus.Fatalf("Cannot connect to usb ftdi on platform '%s'", runtime.GOOS)
	}

	// hooray we fell through, now we can connect to the serial connection and
	//	actually operate the radio
	mode := &serial.Mode{
		BaudRate: 115_200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(serialPath, mode)
	if err != nil {
		logrus.WithError(err).Error("failed to open serial path '%s'")
	}
	defer func() {
		err := port.Close()
		if err != nil {
			logrus.WithError(err).Error("Could not close serial port")
		}
	}()

	cmds := []string{
		"AT+PARAMETER=10,2,1,7",
		"AT+BAND=432500000", // 902300000,
		"AT+ADDRESS=1",
		"AT+NETWORKID=6",
		"AT+CRFOP=15",
	}
	for _, cmd := range cmds {
		resp := sendCommand(port, cmd)
		logrus.Infof("'%s' ran, result: '%s'", cmd, resp)
	}
}

func sendCommand(p serial.Port, cmd string) string {
	_, err := p.Write([]byte(cmd + "\r\n"))
	if err != nil {
		logrus.WithError(err).Fatal("Failed to send message")
	}
	// logrus.Infof("Sent %v bytes: '%s'\n", n, cmd)

	response := make([]byte, 0)
	var n2 uint32
	for {
		lastIter := n2
		n2, err = p.ReadyToRead()
		if err != nil {
			logrus.WithError(err).Error("Could not read response to command.")
		} else if n2 > 0 && lastIter == n2 {
			//logrus.Infof("bytes for reading: %d", n2)
			break
		}
		time.Sleep(20 * time.Millisecond) // TODO can this be removed
	}
	for {
		buf := make([]byte, n2)
		n, err := p.Read(buf)
		if err != nil {
			if strings.Contains(err.Error(), "interrupted") {
				continue
			}
			logrus.WithError(err).Error("Could not read port")
			break
		}
		if n == 0 {
			//logrus.Info("EOM")
			break
		}
		response = append(response, buf...)
	}
	return string(response)
}
