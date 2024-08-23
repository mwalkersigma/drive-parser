package models

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type CostSheetRow struct {
	Manufacturer string
	Model        string
	Condition    int
	Ebay         int
	Notes        string
	AP           bool
	Sku          string
	Inv          int
	ParentSku    string
	CostSentToSV int
}

type CostSheetData struct {
	Values        [][]interface{}
	FormattedRows []CostSheetRow
}

func (c *CostSheetData) Parse() {
OUTER:
	for _, row := range c.Values {
		var newRow CostSheetRow
		// Manufacturer
		newRow.Manufacturer = row[0].(string)

		// Model
		newRow.Model = row[1].(string)

		// Condition
		condition, err := strconv.Atoi(row[3].(string))
		if err != nil {
			fmt.Println("Error converting condition to int")
			continue
		}
		newRow.Condition = condition

		// Ebay
		ebay, err := strconv.Atoi(row[6].(string))
		if err != nil {
			fmt.Println("Error converting ebay to int")
			continue
		}
		newRow.Ebay = ebay

		// Notes
		newRow.Notes = row[8].(string)

		// AP
		ap, err := strconv.ParseBool(row[9].(string))
		if err != nil {
			fmt.Println("Error converting ap to bool")
			continue
		}
		newRow.AP = ap

		// Sku
		newRow.Sku = row[10].(string)
		if row[10] == nil || row[10] == "" {
			continue
		}

		// Inv
		if row[11] != nil && row[11] != "" {
			inv, err := strconv.Atoi(row[11].(string))
			if err != nil {
				fmt.Println("Error converting inv to int")
				fmt.Println("Received: ")
				fmt.Println(row[11].(string))
				continue
			}
			newRow.Inv = inv
		} else {
			newRow.Inv = 0
		}

		// Parent Sku
		newRow.ParentSku = row[14].(string)

		// Cost Sent To SV
		costSentToSV := strings.ReplaceAll(row[15].(string), "$", "")
		costSentToSV = strings.ReplaceAll(costSentToSV, ",", "")
		costSentToSV = strings.TrimSpace(costSentToSV)
		costSentToSVFloat, err := strconv.ParseFloat(costSentToSV, 64)
		// convert to int
		costSentToSVFloat = costSentToSVFloat * 100
		costSentToSVInt := int(math.Round(costSentToSVFloat)) / 100
		if err != nil {
			fmt.Println("Error converting cost sent to sv to int")
			continue
		}
		newRow.CostSentToSV = costSentToSVInt

		// check to see if a row with the same sku already exists
		for _, existingRow := range c.FormattedRows {
			if newRow.Sku == existingRow.Sku {
				fmt.Println("Duplicate Sku: ", newRow.Sku)
				continue OUTER
			}
		}

		c.FormattedRows = append(c.FormattedRows, newRow)
	}
}
