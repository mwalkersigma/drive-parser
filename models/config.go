package models

import (
	"encoding/json"
	"fmt"
	"os"
)

type ConfigJson struct {
	SleepTimeOut int `json:"sleepTimeOut" default:"2"`
}

func createFileIfNotExists(path string) {
	_, err := os.Stat(path)
	if err != nil {
		file, err := os.Create(path)
		if err != nil {
			fmt.Println("Error creating file")
			panic(err)
		}
		emptyData := ConfigJson{SleepTimeOut: 2}
		jsonParser := json.NewEncoder(file)
		err = jsonParser.Encode(emptyData)
		if err != nil {
			fmt.Println("Error saving json")
			panic(err)
		}
	}
}

func (c *ConfigJson) GetConfig() error {
	exists := os.IsExist(os.Mkdir("./json", 0755))
	if exists {
		fmt.Println("Json folder exists")
	}
	createFileIfNotExists("./json/config.json")
	file, err := os.Open("./json/config.json")
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Println("Error closing file")
			panic(err)
		}
	}(file)

	jsonParser := json.NewDecoder(file)
	err = jsonParser.Decode(c)
	if err != nil {
		fmt.Println("Error decoding json")
		return err
	}
	return nil
}
