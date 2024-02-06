/*
	Notes:
	Single word values(1 x uint16) values can be both uint16 and int16.

	Float (2 x uint16) can have endianess swapped at :
	- byte level within a word.
	- word level where each of the uint16's have swapped place.

	Function codes:
	1, read coils
	2, read discrete inputs
	3, read holding registers
	4, read input registers

	Snip from https://www.csimn.com/CSI_pages/Modbus101.html
	--------------------------------------------------------
	Valid address ranges as originally defined for Modbus were 0 to 9999 for each of the above register types. Valid ranges allowed in the current specification are 0 to 65,535. The address range originally supported by Babel Buster gateways was 0 to 9999. The extended range addressing was later added to all new Babel Buster products.
	The address range applies to each type of register, and one needs to look at the function code in the Modbus message packet to determine what register type is being referenced. The Modicon convention uses the first digit of a register reference to identify the register type.
	Register types and reference ranges recognized with Modicon notation are as follows:
	0x = Coil = 00001-09999
	1x = Discrete Input = 10001-19999
	3x = Input Register = 30001-39999
	4x = Holding Register = 40001-49999
	On occasion, it is necessary to access more than 10,000 of a register type. Based on the original convention, there is another de facto standard that looks very similar. Additional register types and reference ranges recognized with Modicon notation are as follows:
	0x = Coil = 000001-065535
	1x = Discrete Input = 100001-165535
	3x = Input Register = 300001-365535
	4x = Holding Register = 400001-465535
	When using the extended register referencing, it is mandatory that all register references be exactly six digits. This is the only way Babel Buster will know the difference between holding register 40001 and coil 40001. If coil 40001 is the target, it must appear as 040001.

	References :
	https://control.com/forums/threads/confused-modbus-tcp-vs-modbus-rtu-over-tcp.37377/
	https://www.simplymodbus.ca/TCP.htm
	https://modbus.org/docs/Modbus_Application_Protocol_V1_1b3.pdf

	TODO:
	- Select what listeners to start, like RTU TCP, Modbus TCP.
	- The name used in the switch/case of the setRegister function is taken from the input fileName. If another fileName if used it will fail. Look into how to make this persistent no matter what filename used.
*/

package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"

	mbserver "github.com/postmannen/modbusgenerator"
)

func main() {
	f := NewFlags()
	f.parseFlags()

	// Start a new server
	serv := mbserver.NewServer()
	err := serv.ListenRTUTCP(f.ListenRTUTCPPort)
	if err != nil {
		log.Printf("%v\n", err)
		return
	}
	defer serv.Close()
	log.Println("Started the modbus generator...")

	// The configuration is split in 4 files, 1 for each register
	//fileNames := []string{f.jsonCoil, f.jsonDiscrete, f.jsonInput, f.jsonHolding}

	// Iterate over all the filenames specified, and create a holding
	// structure to keep all the file handles in, with info about each
	// register.

	configFileSpecified := false

	for _, v := range f.registerFiles {
		if v.filename == "" {
			continue
		}

		configFileSpecified = true

		fh, err := os.Open(v.filename)
		if err != nil {
			log.Printf("error: failed to open config file for %v: %v\n", v.filename, err)
			continue
		}

		config := config{
			name: string(v.registerType),
			fh:   fh,
		}
		defer fh.Close()

		// Since we are using the routine to unmarshall the JSON, and
		// we want it unmarshaled into different types, we use a map
		// with string key and empty interface to store the data values.
		// The converting to the real type it represents is handled in
		// the repsective types Encode method when being called upon.
		//
		registryRawData := []map[string]interface{}{}

		js := json.NewDecoder(config.fh)
		err = js.Decode(&registryRawData)
		//err = json.Unmarshal(js, &objs)
		if err != nil {
			log.Printf("error: decoding json: %v\n", err)
		}

		// registryData will hold all the data to put into a complete
		// register.
		// each element of the slice will represent a register entry.
		var registryData []encoder

		// Since encoder is an interface type, we need to figure out
		// the concrete type each encoder is.
		// Loop over the data unmarshaled above, and call NewEncoder.
		// New encoder will check the obj's type field and return an
		// encoder of the correct concrete type.
		for _, obj := range registryRawData {
			registryData = append(registryData, NewEncoder(obj))
		}

		// setRegister will set and populate the values into the register
		err = setRegister(serv, registryData, string(v.registerType), f.registerStartOffset)
		if err != nil {
			log.Printf("error: setRegister: %v\n", err)
			return
		}

	}

	// If no config files where specified, exit with info message.
	if !configFileSpecified {
		log.Println("info: no config files specified or found. Use the --help flag for how to use the flags.")
		return
	}

	// Wait for someone to press CTRL+C.
	fmt.Println("Press ctrl+c to stop")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	fmt.Println("Stopped")
}

type config struct {
	name string
	fh   *os.File
}

type flags struct {
	// jsonCoil            string
	// jsonDiscrete        string
	// jsonInput           string
	// jsonHolding         string
	registerFiles       []registerFile
	registerStartOffset int
	ListenRTUTCPPort    string
}

func NewFlags() *flags {
	return &flags{}
}

func (f *flags) parseFlags() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Description of flags provided by modbus generator.\n\n")
		flag.PrintDefaults()
	}

	jsonCoil := flag.String("jsonCoil", "", "JSON file to take as input to generate Coil registers")
	jsonDiscrete := flag.String("jsonDiscrete", "", "JSON file to take as input to generate Discrete registers")
	jsonInput := flag.String("jsonInput", "", "JSON file to take as input to generate input registers")
	jsonHolding := flag.String("jsonHolding", "", "JSON file to take as input to generate Holding registers")
	registerStartOffset := flag.Int("registerStartOffset", -1, `Use 0 or -1 (-1 is the default). 
	Do you want the register nr. to be specified as it is in the config file, 
	or to add 1 to the value ? -1 presents it as it is in the config file, 
	or setting the value to 0 will make the generator add 1 to the register 
	address specified in the config. 
	Example: if 0 is specified, a register with the address of 300 in the 
	config file will need to be read as 301 from modpoll.`)
	listenRTUTCPPort := flag.String("listenRTUTCPPort", ":5502", "The address and port to listen on")

	flag.Parse()

	f.registerFiles = append(f.registerFiles, registerFile{filename: *jsonCoil, registerType: coilType})
	f.registerFiles = append(f.registerFiles, registerFile{filename: *jsonDiscrete, registerType: discreteType})
	f.registerFiles = append(f.registerFiles, registerFile{filename: *jsonInput, registerType: inputType})
	f.registerFiles = append(f.registerFiles, registerFile{filename: *jsonHolding, registerType: holdingType})
	f.registerStartOffset = *registerStartOffset
	f.ListenRTUTCPPort = *listenRTUTCPPort
}

type registerType string

const coilType registerType = "coil"
const discreteType registerType = "discrete"
const inputType registerType = "input"
const holdingType registerType = "holding"

type registerFile struct {
	filename     string
	registerType registerType
}

// uint16ToLittleEndian will swap the byte order of the 'two
// 8 bit bytes that an uint16 is made up of.
func uint16ToLittleEndian(u uint16) uint16 {
	fmt.Printf("before: %b\n", u)
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, u)
	v := binary.BigEndian.Uint16(b)
	fmt.Printf("after: %b\n", v)
	return v
}

// uint16ToByteSlice will split a 16 bit value into two 8 bit
// values, and return them as a slice if bytes.
func uint16ToByteSlice(u uint16) []byte {
	var b []byte
	u1 := byte(u >> 8)
	u2 := byte(u & 0xFF)
	b = append(b, u1, u2)

	return b
}

// The size of the registers in number of uint16's
const coilSize = 1
const discreteSize = 1
const inputSize = 2
const holdingSize = 2

// setRegister will set the values into the register that is presented as a slice
// within the serv receiver.
func setRegister(serv *mbserver.Server, registryData []encoder, registerType string, addrOffset int) error {
	var prevAddr int

	switch registerType {
	case "coil":
		for _, v := range registryData {
			b := uint16ToByteSlice(v.Encode()[0])
			addr := v.Address() + addrOffset

			if prevAddr > addr-coilSize {
				return fmt.Errorf("wrong increment of address in coil register for address after %v", addr)
			}

			serv.Coils = append(serv.Coils[:addr], b...)
			prevAddr = addr
		}
	case "discrete":
		for _, v := range registryData {
			b := uint16ToByteSlice(v.Encode()[0])
			addr := v.Address() + addrOffset

			if prevAddr > addr-discreteSize {
				return fmt.Errorf("wrong increment of address in discrete register for address after %v", addr)
			}

			serv.DiscreteInputs = append(serv.DiscreteInputs[:addr], b...)
			prevAddr = addr
		}
	case "input":
		for _, v := range registryData {
			addr := v.Address() + addrOffset

			if prevAddr > addr-inputSize {
				return fmt.Errorf("wrong increment of address in input register for address after %v", addr)
			}

			serv.InputRegisters = append(serv.InputRegisters[:addr], v.Encode()...)
			prevAddr = addr
		}
	case "holding":
		for _, v := range registryData {
			addr := v.Address() + addrOffset

			if prevAddr > addr-holdingSize {
				return fmt.Errorf("wrong increment of address in holding register for address after %v", addr)
			}

			serv.HoldingRegisters = append(serv.HoldingRegisters[:addr], v.Encode()...)
			prevAddr = addr
		}
	default:
		return fmt.Errorf("wrong file given: Allowed files are coil.json|discrete.json|input.json|holding.json")
	}

	return nil
}

// -----------------------------------Encoder's----------------------------------------

// encoder represent any value type that can be encoded
// into a []uint16 as a response back to the modbus request.
type encoder interface {
	Encode() []uint16
	Address() int
}

type float32LittleWordBigEndian struct {
	Type    string
	Number  float64
	RegAddr float64
}

// encode will encode a float32 value into []uint16 where:
//   - The two 16 bits word are little endian
//   - The Byte order of each word a big endian
func (f float32LittleWordBigEndian) Encode() []uint16 {
	n := float32(f.Number)
	v1 := uint16((math.Float32bits(n) >> 16) & 0xffff)
	v2 := uint16((math.Float32bits(n)) & 0xffff)
	// fmt.Printf("*v1 = %v*\n", v1)
	return []uint16{v2, v1}
}

func (f float32LittleWordBigEndian) Address() int {
	n := int(f.RegAddr)
	return n
}

// -------

type float32BigWordBigEndian struct {
	Type    string
	Number  float64
	RegAddr float64
}

// encode will encode a float32 value into []uint16 where:
//   - The two 16 bits word are little endian
//   - The Byte order of each word a big endian
func (f float32BigWordBigEndian) Encode() []uint16 {
	n := float32(f.Number)
	v1 := uint16((math.Float32bits(n) >> 16) & 0xffff)
	v2 := uint16((math.Float32bits(n)) & 0xffff)
	// fmt.Printf("*v1 = %v*\n", v1)
	return []uint16{v1, v2}
}

func (f float32BigWordBigEndian) Address() int {
	n := int(f.RegAddr)
	return n
}

// -------

type float32LittleWordLittleEndian struct {
	Type    string
	Number  float64
	RegAddr float64
}

// encode will encode a float32 value into []uint16 where:
//   - The two 16 bits word are little endian
//   - The Byte order of each word a big endian
func (f float32LittleWordLittleEndian) Encode() []uint16 {
	n := float32(f.Number)
	v1 := uint16((math.Float32bits(n) >> 16) & 0xffff)
	v2 := uint16((math.Float32bits(n)) & 0xffff)
	// fmt.Printf("*v1 = %v*\n", v1)

	v1 = uint16ToLittleEndian(v1)
	v2 = uint16ToLittleEndian(v2)
	return []uint16{v2, v1}
}

func (f float32LittleWordLittleEndian) Address() int {
	n := int(f.RegAddr)
	return n
}

// -------

type float32BigWordLittleEndian struct {
	Type    string
	Number  float64
	RegAddr float64
}

// encode will encode a float32 value into []uint16 where:
//   - The two 16 bits word are little endian
//   - The Byte order of each word a big endian
func (f float32BigWordLittleEndian) Encode() []uint16 {
	n := float32(f.Number)
	v1 := uint16((math.Float32bits(n) >> 16) & 0xffff)
	v2 := uint16((math.Float32bits(n)) & 0xffff)
	// fmt.Printf("*v1 = %v*\n", v1)

	v1 = uint16ToLittleEndian(v1)
	v2 = uint16ToLittleEndian(v2)
	return []uint16{v1, v2}
}

func (f float32BigWordLittleEndian) Address() int {
	n := int(f.RegAddr)
	return n
}

// -------

type wordInt16BigEndian struct {
	Type    string
	Number  float64
	RegAddr float64
}

func (w wordInt16BigEndian) Encode() []uint16 {
	// NB: The type wordInt16BigEndian with it's decode method
	// is primarily used for coils and discrete registers.
	// In other implementations it seems like coil value is held
	// in the 8MSB, and that the value is set to 0x1 for the 8 LSB,
	// but the reason for this I've not completely understood.
	// So beware that this one might be wrong implemented and might
	// need to be changed.
	// But the modpoll tool seems to interpret the value returned
	// from the register ok as it is, so it seems to be good.

	//v := uint16(w.Number)
	vTmp := uint16(w.Number) << 8
	v := vTmp | uint16(1)

	return []uint16{v}
}

func (f wordInt16BigEndian) Address() int {
	return int(f.RegAddr)
}

// -------

type wordInt16LittleEndian struct {
	Type    string
	Number  float64
	RegAddr float64
}

func (w wordInt16LittleEndian) Encode() []uint16 {
	v := uint16(w.Number)
	v = uint16ToLittleEndian(v)

	return []uint16{v}
}

func (f wordInt16LittleEndian) Address() int {
	return int(f.RegAddr)
}

// -------------------------------------------------------------------------

// NewEncoder will take the raw data given to it,
// check the "type" field, and return a decoder
// with the type set based on the "type" field.
func NewEncoder(m map[string]interface{}) encoder {
	switch m["type"].(string) {
	case "float32LittleWordBigEndian":
		return NewFloat32LittleWordBigEndian(m)
	case "float32BigWordBigEndian":
		return NewFloat32BigWordBigEndian(m)
	case "float32LittleWordLittleEndian":
		return NewFloat32LittleWordLittleEndian(m)
	case "float32BigWordLittleEndian":
		return NewFloat32BigWordLittleEndian(m)
	case "wordInt16BigEndian":
		return NewWordInt16BigEndian(m)
	case "wordInt16LittleEndian":
		return NewWordInt16LittleEndian(m)
	}
	return nil
}

// Create the concrete types for the interface type enocoder.
//
// Since we are taking the value types in as interface{} only float64's
// will be allowed in the JSON, and since it is an interface type we assert
// it to an float64, but we convert it to it's correct type in the encode
// method for each concrete type, e.g. uint16.

// NewFloat32LittleWordBigEndian will assert the struct fields to it's
// correct type, and return the concrete type.
func NewFloat32LittleWordBigEndian(m map[string]interface{}) *float32LittleWordBigEndian {
	return &float32LittleWordBigEndian{
		Type:    m["type"].(string),
		Number:  m["number"].(float64),
		RegAddr: m["regAddr"].(float64),
	}
}

// NewFloat32BigWordBigEndian will assert the struct fields to it's
// correct type, and return the concrete type.
func NewFloat32BigWordBigEndian(m map[string]interface{}) *float32BigWordBigEndian {
	return &float32BigWordBigEndian{
		Type:    m["type"].(string),
		Number:  m["number"].(float64),
		RegAddr: m["regAddr"].(float64),
	}
}

// NewFloat32LittleWordLittleEndian will assert the struct fields to it's
// correct type, and return the concrete type.
func NewFloat32LittleWordLittleEndian(m map[string]interface{}) *float32LittleWordLittleEndian {
	return &float32LittleWordLittleEndian{
		Type:    m["type"].(string),
		Number:  m["number"].(float64),
		RegAddr: m["regAddr"].(float64),
	}
}

// NewFloat32BigWordLittleEndian will assert the struct fields to it's
// correct type, and return the concrete type.
func NewFloat32BigWordLittleEndian(m map[string]interface{}) *float32BigWordLittleEndian {
	return &float32BigWordLittleEndian{
		Type:    m["type"].(string),
		Number:  m["number"].(float64),
		RegAddr: m["regAddr"].(float64),
	}
}

// NewWordInt16BigEndian will assert the struct fields to it's
// correct type, and return the concrete type.
func NewWordInt16BigEndian(m map[string]interface{}) *wordInt16BigEndian {
	return &wordInt16BigEndian{
		Type:    m["type"].(string),
		Number:  m["number"].(float64),
		RegAddr: m["regAddr"].(float64),
	}
}

// NewWordInt16LittleEndian will assert the struct fields to it's
// correct type, and return the concrete type.
func NewWordInt16LittleEndian(m map[string]interface{}) *wordInt16LittleEndian {
	return &wordInt16LittleEndian{
		Type:    m["type"].(string),
		Number:  m["number"].(float64),
		RegAddr: m["regAddr"].(float64),
	}
}
