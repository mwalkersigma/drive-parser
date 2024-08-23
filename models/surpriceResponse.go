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
	defer response.Body.Close()
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
