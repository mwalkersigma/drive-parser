package models

import (
	"net/http"
	"io"
	"fmt"
	"encoding/json"
)
type SurpriceResponse struct {
	Data []struct {
		Manufacturer string  `json:"manufacturer"`
		Model        string  `json:"model"`
		Sku          any     `json:"Sku"`
		Cost         float64 `json:"Cost"`
		Quantity     string  `json:"Quantity"`
		Price        json.Number  `json:"Price"`
		Condition    string  `json:"Condition"`
		TotalCost    int     `json:"totalCost"`
	} `json:"data"`
	IsSubmitted bool   `json:"isSubmitted"`
	Title       string `json:"title"`
	Cost        int    `json:"cost"`
}
func (this *SurpriceResponse) JSON(response *http.Response) error {
	var ResponseJson SurpriceResponse
	body, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Println("Error reading response body")
		return err
	}
	defer response.Body.Close()
	if err := json.Unmarshal(body, &ResponseJson); err != nil {
		fmt.Println("Error unmarshalling response body")
		fmt.Println(err)
		return err
	}
	*this = ResponseJson
	return nil
}