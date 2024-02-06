# Modbus simulator

Modbus simulator will let you start a modbus slave with pregenerated dataset's that will be read from json files provided by the caller.

## Run / build

In an environment where Go is installed.

To run the code directly

```bash
go run main.go --help
```

To build an executable for Linux amd64 architecture.

```bash
env GOOS=linux GOARCH=amd64 go build
```

## Precombiled executable binary for Linux amd64

`modbusgenerator`

## JSON config file

The config for a specific register are specified in a JSON file. Each type of register coil|discrete|input|handler must be specified in it's own separate config file, so for example a coil register must be specified in it's own file, a discrete register in it's own file, and so on.

An example of a JSON config file for a holding register.

```json
[{
    "type": "float32LittleWordBigEndian",
    "number": 3.1415,
    "regAddr": 101
}, {
    "type": "float32BigWordBigEndian",
    "number": 3.1415926,
    "regAddr": 103
}, {
    "type": "float32BigWordBigEndian",
    "number": 3.101010,
    "regAddr": 105
}]
```

The general structure are to specify one or more elements where each element describes a single address in the specific register.

Explanation of the elements:

- type:
  There are in general 6 types to choose from:

  - float32LittleWordBigEndian
    Value of 2 x uint16, where the two uints have swapp'ed order, and the byte order within each uint is in normal order.
  - float32BigWordBigEndian
    Value of 2 x uint16, where the two uints are in normal order, and the byte order within each uint is in normal order.
  - float32LittleWordLittleEndian
    Value of 2 x uint16, where the two uints have swapp'ed order, and the byte order within each uint is in swap'ed order.
  - float32BigWordLittleEndian
    Value of 2 x uint16, where the two uints are in normal order, and the byte order within each uint is in swap'ed order.
  - wordInt16BigEndian
    Value of a single uint16, where the byte order is in normal order.
    This is generally used for coil and discrete registers.
  - wordInt16LittleEndian
    Value of a single uint16, where the byte order is in swap'ed order.
    Generally not used.

Numbers for :

- input and holding registers are float values.
- coil and discrete values are 0 or 1.

regAddr are integer values representing the address number.

## Flags provided by the modbus simulator

```bash
Description of flags provided by modbus generator.

  -jsonCoil string
        JSON file to take as input to generate Coil registers
  -jsonDiscrete string
        JSON file to take as input to generate Discrete registers
  -jsonHolding string
        JSON file to take as input to generate Holding registers
  -jsonInput string
        JSON file to take as input to generate input registers
  -listenRTUTCPPort string
        The address and port to listen on (default ":5502")
  -registerStartOffset int
        Use 0 or -1 (-1 is the default). 
                Do you want the register nr. to be specified as it is in the config file, 
                or to add 1 to the value ? -1 presents it as it is in the config file, 
                or setting the value to 0 will make the generator add 1 to the register 
                address specified in the config. 
                Example: if 0 is specified, a register with the address of 300 in the 
                config file will need to be read as 301 from modpoll. (default -1)
```
