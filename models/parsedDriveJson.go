package models

import (
	"encoding/json"
	"fmt"
	"os"
)

type ParsedDrivesJson struct {
	Drives []string `json:"drives"`
}

func (p *ParsedDrivesJson) GetDrives() {
	// check if json folder exists
	exists := os.IsExist(os.Mkdir("./json", 0755))
	if exists {
		fmt.Println("Json folder exists")
	}
	// check if parsedFiles.json exists
	_, err := os.Stat("./json/parsedFiles.json")
	if err != nil {
		// create the file { "drives": [] }
		file, err := os.Create("./json/parsedFiles.json")
		if err != nil {
			fmt.Println("Error creating file")
			panic(err)
		}
		defer func(file *os.File) {
			err := file.Close()
			if err != nil {
				fmt.Println("Error closing file")
				panic(err)
			}
		}(file)
		emptyData := ParsedDrivesJson{Drives: []string{}}
		jsonParser := json.NewEncoder(file)
		err = jsonParser.Encode(emptyData)
		if err != nil {
			fmt.Println("Error saving json")
			panic(err)
		}
	}

	file, err := os.Open("./json/parsedFiles.json")
	if err != nil {
		fmt.Println("Error opening file")
		panic(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Println("Error closing file")
			panic(err)
		}
	}(file)

	jsonParser := json.NewDecoder(file)
	err = jsonParser.Decode(p)
	if err != nil {
		fmt.Println("Error parsing json")
		panic(err)
	}
}
func (p *ParsedDrivesJson) HasDrive(drive string) bool {
	for _, d := range p.Drives {
		if d == drive {
			return true
		}
	}
	return false
}
func (p *ParsedDrivesJson) AddDrive(drive string) {
	p.Drives = append(p.Drives, drive)
}
func (p *ParsedDrivesJson) RemoveDrive(drive string) {
	for i, d := range p.Drives {
		if d == drive {
			p.Drives = append(p.Drives[:i], p.Drives[i+1:]...)
		}
	}
}
func (p *ParsedDrivesJson) SaveDrives() {
	file, err := os.Create("./json/parsedFiles.json")
	if err != nil {
		fmt.Println("Error creating file")
		panic(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Println("Error closing file")
			panic(err)
		}
	}(file)
	jsonParser := json.NewEncoder(file)
	err = jsonParser.Encode(p)
	if err != nil {
		fmt.Println("Error saving json")
		panic(err)
	}
}
