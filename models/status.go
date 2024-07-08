package models

import (
	"encoding/json"
	"fmt"
	"os"
)

type Status struct {
	Running bool `json:"running"`
}

func (p *Status) GetStatus() {
	// check if json folder exists
	exists := os.IsExist(os.Mkdir("./json", 0755))
	if exists {
		fmt.Println("Json folder exists")
	}
	// check if parsedFiles.json exists
	_, err := os.Stat("./json/status.json")
	if err != nil {
		// create the file { "drives": [] }
		file, err := os.Create("./json/status.json")
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
		emptyData := Status{Running: true}
		jsonParser := json.NewEncoder(file)
		err = jsonParser.Encode(emptyData)
		if err != nil {
			fmt.Println("Error saving json")
			panic(err)
		}
	}

	file, err := os.Open("./json/status.json")
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
func (p *Status) Save() {
	file, err := os.Create("./json/status.json")
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
