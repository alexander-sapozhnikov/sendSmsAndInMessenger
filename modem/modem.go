package modem

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/tarm/serial"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)
var lock sync.Mutex
const waitReps int = 5
// ответы получаемые от модема
const (
	SMSStatusOk = "OK"
	SMSStatusError = "Error"
)
type GSMModem struct {
	ComPort  string
	BaudRate int
	Port     *serial.Port
	DeviceId string
}

func New(ComPort string, BaudRate int, DeviceId string) (modem *GSMModem) {
	modem = &GSMModem{ComPort: ComPort, BaudRate: BaudRate, DeviceId: DeviceId}
	return modem
}

func (m *GSMModem) Connect() (err error) {
	config := &serial.Config{Name: m.ComPort, Baud: m.BaudRate, ReadTimeout: time.Second}
	m.Port, err = serial.OpenPort(config)

	if err == nil {
		m.initModem()
	}

	return err
}

func (m *GSMModem) initModem() {
	m.SendCommand("ATE0\r\n", true) // echo off
	m.SendCommand("AT+CMEE=1\r\n", true) // useful error messages
	m.SendCommand("AT+WIND=0\r\n", true) // disable notifications
}

func (m *GSMModem) ExpectAnswer() (string, error) {

	var status string
	var buffer bytes.Buffer
	buf := make([]byte, 32)
	lock.Lock()
	defer lock.Unlock()
	for i := 1; i < waitReps+1; {
		// ignoring error as EOF raises error on Linux
		n, _ := m.Port.Read(buf)
		if n > 0 {
			buffer.Write(buf[:n])
			status = buffer.String()
			log.Printf("WaitForOutput: received %d bytes: %#v\n", n, string(buf[:n]))
			if strings.Contains(status, SMSStatusOk) {
				return SMSStatusOk, nil
			} else if strings.Contains(status, SMSStatusError) {
				errorCodes := regexp.MustCompile(`([A-Z ]*)ERROR([0-9A-Za-z :]*)`).FindAllStringSubmatch(status, -1)
				if errorCodes[0][1] == "" && errorCodes[0][2] == "" {
					return status, fmt.Errorf("WaitForOutput: Found unknown ERROR")
				} else {
					return status, fmt.Errorf("WaitForOutput: Found %vERROR%v", errorCodes[0][1], errorCodes[0][2])
				}
			}
		} else {
			log.Printf("WaitForOutput: No output on %dth iteration", i)
			i++
		}
	}
	return status, errors.New("Error. WaitForOutput: Timed out.")
}

func (m *GSMModem) Send(command string) {
	log.Println("--- Send:", m.transposeLog(command))
	m.Port.Flush()
	_, err := m.Port.Write([]byte(command))
	if err != nil {
		log.Fatal(err)
	}
}

func (m *GSMModem) Read(n int) string {
	var output string = "";
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		// ignoring error as EOF raises error on Linux
		c, _ := m.Port.Read(buf)
		if c > 0 {
			output = string(buf[:c])
		}
	}

	log.Printf("--- Read(%d): %v", n, m.transposeLog(output))
	return output
}

func (m *GSMModem) SendCommand(command string, waitForOk bool) string {
	m.Send(command)

	if waitForOk {
		output, _ := m.ExpectAnswer() // we will not change api so errors are ignored for now
		return output
	} else {
		return m.Read(1)
	}
}

// hexUTF16FromString кодирует в USC-2
func hexUTF16FromString(s string) string {
	hex := fmt.Sprintf("%04x", utf16.Encode([]rune(s)))
	return strings.Replace(hex[1:len(hex)-1], " ", "", -1)
}

// convertMobile кодирует номер в pdu
func convertMobile(number string) string{
	if number[0] == '+'{
		number = number[1:]
	}
	if number[0] == '8'{
		number = "7" + number[1:]
	}

	if len([]rune(number)) % 2 != 0{
		number += "F"
	}

	numberArray := strings.Split(number, "")
	for i := 0; i < len(numberArray); i +=2 {
		numberArray[i], numberArray[i+1] = numberArray[i+1], numberArray[i]
	}

	return strings.Join(numberArray, "")
}

// convertCountMessage кодирует длину сообщения в pdu
func convertCountMessage(message string) string{
	lenMessage := strconv.FormatInt(int64(len([]rune(message))*2), 16)

	if len([]rune(lenMessage)) == 1 {
		lenMessage = "0" + lenMessage
	}

	return lenMessage
}

func (m *GSMModem) SendSMS(mobile string, message string) string {
	log.Println("--- SendSMS ", mobile, message)

	m.SendCommand("AT+CMGF=0\r", true)

	message = fmt.Sprintf("0001000B91%s0008%s%s", convertMobile(mobile), convertCountMessage(message), hexUTF16FromString(message))

	lengthMessage := len([]rune(message))
	m.SendCommand(fmt.Sprintf("AT+CMGS=%d\r", (lengthMessage -2) / 2, true), true)

	// EOM CTRL-Z = 26
	return m.SendCommand(message+string(26), true)
}

func (m *GSMModem) transposeLog(input string) string {
	output := strings.Replace(input, "\r\n", "\\r\\n", -1);
	return strings.Replace(output, "\r", "\\r", -1);
}
