package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"github.com/mwalkersigma/drive-parser/models"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"net/http"
	"os"
	"strings"
)

//go:embed sheetCert.json
var sheetCert []byte

type Item struct {
	Sku  string  `json:"Sku"`
	Cost float64 `json:"Cost"`
}

func (i *Item) String() string {
	return fmt.Sprintf(`{"Sku": "%s", "Cost": %f}`, i.Sku, i.Cost)
}

type SVRequestBody struct {
	Items       []Item `json:"Items"`
	UserToken   string `json:"UserToken"`
	TenantToken string `json:"TenantToken"`
}

func (r *SVRequestBody) ToJSON() string {
	var stringified []string
	for _, item := range r.Items {
		stringified = append(stringified, item.String())
	}
	return fmt.Sprintf(`{"Items": [%v], "UserToken": "%s", "TenantToken": "%s"}`, strings.Join(stringified, ","), r.UserToken, r.TenantToken)
}

var sheetsService *sheets.Service
var p models.ParsedDrivesJson

func init() {
	p.GetDrives()
	ctx := context.Background()

	ss, err := sheets.NewService(ctx, option.WithCredentialsJSON(sheetCert))
	if err != nil {
		fmt.Println("Error creating new service")
		panic(err)
	}

	sheetsService = ss
}

func getSheetId(url string) string {
	return strings.Split(strings.SplitAfter(url, "/d/")[1], "/edit")[0]
}

func main() {

	// get a url from the user
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the url of the cost sheet: ")
	url, _ := reader.ReadString('\n')
	url = strings.TrimSpace(url)
	// get the cost sheet id.

	sheetId := getSheetId(url)

	fmt.Println("Sheet ID: ", sheetId)

	// get the cost sheet data
	costSheetData, err := sheetsService.
		Spreadsheets.Values.
		Get(sheetId, "Offer Template!A:P").
		Do()
	if err != nil {
		fmt.Println("Error getting cost sheet data")
		panic(err)
	}
	// filter out rows with no data
	costSheetData.Values = costSheetData.Values[1:]
	fmt.Println("Filtering out empty rows")
	for j := 0; j < len(costSheetData.Values); j++ {
		row := costSheetData.Values[j]
		if row[0] == "" {
			costSheetData.Values = append(costSheetData.Values[:j], costSheetData.Values[j+1:]...)
			j--
		}
	}
	expectedLength := len(costSheetData.Values)
	fmt.Println("Filtering complete.")

	if len(costSheetData.Values) == 0 {
		fmt.Println("No data found in the cost sheet")
		fmt.Println("Press Enter to exit")
		_, _ = reader.ReadString('\n')
		return
	}
	//fmt.Println("Cost Sheet Data: ", costSheetData.Values)
	var sheetData models.CostSheetData
	sheetData.Values = costSheetData.Values
	sheetData.Parse()
	if len(sheetData.FormattedRows) != expectedLength {
		fmt.Println("Missing Sku Values")
		fmt.Printf("Expected Length: %d Recieved: %d", expectedLength, len(sheetData.FormattedRows))
		fmt.Println("This is likely cause by a duplicate sku in the cost sheet")
		//continue
	}

	var items []Item
	for _, row := range sheetData.FormattedRows {
		var item Item
		item.Sku = row.Sku
		item.Cost = float64(row.CostSentToSV)
		items = append(items, item)
	}
	fmt.Printf("Sending %d items to SkuVault\n", len(items))
	updateUrl := "https://app.skuvault.com/api/products/updateProducts"
	if len(items) < 100 {
		var requestBody SVRequestBody
		requestBody.Items = items
		requestBody.UserToken = "aev/tZ/ZhRsA/h/C8PKpZXR9iTWQDqJ8+Nztj8B3mTc="
		requestBody.TenantToken = "FsoVOQznBeUrR5188WQqkxrt5o8ZE/64OLHY2/LASZE="
		body := requestBody.ToJSON()
		response, err := http.Post(updateUrl, "application/json", strings.NewReader(body))
		if err != nil {
			fmt.Println("Error updating items")
			panic(err)
		}
		fmt.Println("Items Sent to SkuVault successfully")
		fmt.Println("Response: ", response)
		// print out the response body
		fmt.Printf("Response Body: %s\n", response.Body)

	} else {
		// split the items into chunks of 100
		fmt.Println("Items Length: ", len(items))
		var chunkedItems [][]Item
		for i := 0; i < len(items); i += 100 {
			end := i + 100
			if end > len(items) {
				end = len(items)
			}
			chunkedItems = append(chunkedItems, items[i:end])
		}

		for _, chunk := range chunkedItems {
			var requestBody SVRequestBody
			requestBody.Items = chunk
			requestBody.UserToken = "aev/tZ/ZhRsA/h/C8PKpZXR9iTWQDqJ8+Nztj8B3mTc="
			requestBody.TenantToken = "FsoVOQznBeUrR5188WQqkxrt5o8ZE/64OLHY2/LASZE="
			body := requestBody.ToJSON()
			response, err := http.Post(updateUrl, "application/json", strings.NewReader(body))
			if err != nil {
				fmt.Println("Error updating items")
				panic(err)
			}
			fmt.Println("Response: ", response)
		}
		fmt.Println("Items Sent to SkuVault successfully")
	}
	fmt.Println("Script Execution Complete.")
	fmt.Println("Press Enter to exit")
	_, _ = reader.ReadString('\n')
}
