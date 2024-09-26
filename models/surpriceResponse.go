package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Component struct {
	Manufacturer any     `json:"manufacturer"`
	Model        any     `json:"model"`
	Sku          any     `json:"Sku"`
	Cost         float64 `json:"Cost"`
	Quantity     any     `json:"Quantity"`
	Price        any     `json:"Price"`
}

type SurpriceResponse struct {
	Data        []Component `json:"data"`
	IsSubmitted bool        `json:"isSubmitted"`
	Title       string      `json:"title"`
	Cost        int         `json:"cost"`
	Error       bool        `json:"error"`
}

func (this *SurpriceResponse) JSON(response *http.Response) error {
	var ResponseJson SurpriceResponse
	body, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error reading response body")
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body")
		}
	}(response.Body)
	if err := json.Unmarshal(body, &ResponseJson); err != nil {
		// print out the body for debugging
		fmt.Println(string(body))
		fmt.Println("Error unmarshalling response body")
		fmt.Println(err)
		return err
	}
	*this = ResponseJson
	return nil
}

type Item struct {
	Manufacturer string      `json:"manufacturer"`
	Model        string      `json:"model"`
	Sku          interface{} `json:"Sku"`
	Cost         float64     `json:"Cost"`
	Quantity     string      `json:"Quantity"`
	Price        any         `json:"Price"`
	Condition    string      `json:"Condition"`
}

type DriveParserData struct {
	PoNumber         string     `json:"poNumber"`
	PoResponseStatus string     `json:"poResponseStatus"`
	Items            []struct{} `json:"items"`
	BadItems         []Item     `json:"badItems"`
}

func (d DriveParserData) String() string {
	baseString := fmt.Sprintf("PO Number: %s\nPO Response Status: %s\nItems: %v\nBad Items: [ \n", d.PoNumber, d.PoResponseStatus, d.Items)
	for _, item := range d.BadItems {
		baseString += fmt.Sprintf("{ Manufacturer: %s,\nModel: %s,\nSku: %v,\nCost: %f,\nQuantity: %s,\nPrice: %s,\nCondition: %s,\n } \n", item.Manufacturer, item.Model, item.Sku, item.Cost, item.Quantity, item.Price, item.Condition)
	}
	baseString += " ]\n"
	return baseString
}

type DriveParserResponse struct {
	Error   bool            `json:"error"`
	Message string          `json:"message"`
	Data    DriveParserData `json:"data"`
}

func (d DriveParserResponse) String() string {
	return fmt.Sprintf("Error: %t\nMessage: %s\nData: %v", d.Error, d.Message, d.Data.String())
}

type DriveStatusData struct {
	PoCreationDate     string `json:"po_creation_date"`
	PoCreationStatus   bool   `json:"po_creation_status"`
	SheetId            string `json:"sheet_id"`
	SheetName          string `json:"sheet_name"`
	IsReviewed         bool   `json:"is_reviewed"`
	WhoReviewed        string `json:"who_reviewed"`
	PoCreatedBy        string `json:"po_created_by"`
	SheetFailureReason string `json:"sheet_failure_reason"`
}

func (d DriveStatusData) String() string {
	return fmt.Sprintf("PO Creation Date: %s\nPO Creation Status: %s\nSheet ID: %s\nSheet Name: %s\nIs Reviewed: %t\nWho Reviewed: %s\nPO Created By: %s\n", d.PoCreationDate, d.PoCreationStatus, d.SheetId, d.SheetName, d.IsReviewed, d.WhoReviewed, d.PoCreatedBy)
}

type DriveStatusResponse struct {
	Error   bool            `json:"error"`
	Message string          `json:"message"`
	Data    DriveStatusData `json:"data"`
}

func (d DriveStatusResponse) String() string {
	return fmt.Sprintf("Error: %t\nMessage: %s\nData: %v", d.Error, d.Message, d.Data.String())
}
