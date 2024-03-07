package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/mwalkersigma/drive-parser/modules"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

var retroCostingTemplateID = `1ZLO39C95sDUWPsKfGORIGuw8Ep-oJ5VJ2HCce0i2NM4`
var procurementFolderID = `1TeXMYU9jzWZyna7zB8jngeirvhJosvdO`

var timeout = 5

var surpriceURLGetCost, surpriceURLUpdateCost string
var driveService *drive.Service
var sheetsService *sheets.Service

type SurpriceResponse struct {
	Data []struct {
		Manufacturer string `json:"manufacturer"`
		Model        string `json:"model"`
		Sku          any    `json:"Sku"`
		Cost         any    `json:"Cost"`
		Quantity     any    `json:"Quantity"`
		Price        int    `json:"Price"`
		Condition    any    `json:"Condition"`
		TotalCost    int    `json:"totalCost"`
	} `json:"data"`
	Title string `json:"title"`
	Cost  int    `json:"cost"`
}

func countDownTimer(duration int){
	for i := duration; i > 0; i-- {
		// print on the same line
		fmt.Printf("\rSleeping for %d seconds", i)
		time.Sleep(time.Second)
	}

}

func init() {

	fmt.Println(" Starting Costing Sheet Generator And Parser... ")

	err := godotenv.Load()
	if err != nil {
		panic(err)
	}

	surpriceURLGetCost = fmt.Sprintf("%s/api/getCostsFromSheet", os.Getenv("BASE_URL"))
	surpriceURLUpdateCost = fmt.Sprintf("%s/api/updateCostSkuVault", os.Getenv("BASE_URL"))

	ctx := context.Background()
	ds, err := drive.NewService(ctx, option.WithCredentialsFile("./cert.json"))
	if err != nil {
		fmt.Println("Error creating new service")
		panic(err)
	}
	driveService = ds

	fmt.Println("Service created successfully")
	fmt.Println("Retro Costing Template ID: ", retroCostingTemplateID)
	fmt.Println("Procurement Folder ID: ", procurementFolderID)

	ss, err := sheets.NewService(ctx, option.WithCredentialsFile("./SheetCert.json"))
	if err != nil {
		fmt.Println("Error creating new service")
		panic(err)
	}

	sheetsService = ss
}

func ShouldBeSentToCost(sheetID string) (cost int) {
	var sheetAcceptedOffer string = "T3"
	sheetRange := fmt.Sprintf("Final Offer!%s", sheetAcceptedOffer)
	fmt.Println("Sheet Range: ", sheetRange)
	resp, err := sheetsService.Spreadsheets.Values.Get(sheetID, sheetRange).Do()
	if err != nil {
		fmt.Println("Error getting sheet")
		fmt.Println("Sheet ID: ", sheetID)
		fmt.Println(err)
		panic(err)
	}
	fmt.Println("Response: ", modules.PrettyPrint(resp))
	if  len(resp.Values) < 1 {
		fmt.Println("No data found in row")
		return 0
	}
	if len(resp.Values[0]) < 1 {
		fmt.Println("No data found in cell")
		return 0
	}
	fmt.Println("Data: ", resp.Values[0][0])

	currencyStr := resp.Values[0][0].(string)
	fmt.Println("Currency String: ", currencyStr)
	currencyStr = strings.Split(currencyStr, "$")[1]
	currencyStr = strings.Replace(currencyStr, ",", "", -1)
	
	fmt.Println("Currency String: ", currencyStr)
	cost,err = strconv.Atoi(currencyStr)
	if err != nil {
		fmt.Println("Error converting currency string to int")
		fmt.Println(err)
		panic(err)
	}

	return cost
}

func CreateCostSheet(sheetID string, parentFolderId string, cost int) (respId string, err error) {
	costDataRange := "A2:D"
	costDataRange = fmt.Sprintf("Final Offer!%s", costDataRange)

	costData, err := sheetsService.Spreadsheets.Values.Get(sheetID, costDataRange).Do()
	if err != nil {
		fmt.Println("Error getting sheet")
		fmt.Println("Sheet ID: ", sheetID)
		fmt.Println(err)
		return "", err
	}

	resp,err := driveService.Files.Copy(retroCostingTemplateID, &drive.File{
		Name: fmt.Sprintf("Cost Sheet - %s", time.Now().Format("2006-01-02")),
		Parents: []string{parentFolderId},
	}).Do()
	if err != nil {
		fmt.Println("Error copying file")
		fmt.Println(err)
		return "", err
	}

	fmt.Println("File copied successfully")
	fmt.Println("File ID: ", resp.Id)
	copyDataRange := "A2:D"
	copyDataRange = fmt.Sprintf("Offer Template!%s", copyDataRange)
	
	costData.Range = copyDataRange

	update,err := sheetsService.Spreadsheets.Values.Update(resp.Id, copyDataRange, costData).ValueInputOption("RAW").Do()
	if err != nil {
		fmt.Println("Error updating sheet")
		fmt.Println(err)
		return "", err
	}
	fmt.Println("Update: ", update)
	fmt.Println("Cost data updated successfully")

	update,err = sheetsService.Spreadsheets.Values.Update(resp.Id, "Offer Template!S3", &sheets.ValueRange{
		Values: [][]interface{}{{cost}},
		Range: "Offer Template!S3",
		MajorDimension: "ROWS",
	}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		fmt.Println("Error updating cost on sheet")
		fmt.Println(err)
		return "", err
	}
	fmt.Println("Update: ", update)
	fmt.Println("Cost updated successfully")

	return resp.Id, nil
}

func main() {
	fmt.Println("Starting main function")
	defer fmt.Println("Main function finished")
	files, err := driveService.
		Files.
		List().
		Q(fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder'", procurementFolderID)).
		Do()
	if err != nil {
		fmt.Println("Error getting files from folder")
		panic(err)
	}
	jobs, results, wg := modules.SetupWorkers(10)
	fmt.Println("Jobs, Results and WaitGroup created successfully")

	for _, file := range files.Files {
		slices := strings.Split(file.Name, "-")
		if len(slices) > 2 {
			fmt.Println("File Name: ", file.Name)
			jobs <- file.Id
		} else {
			continue
		}
	}

	close(jobs)
	fmt.Println("Jobs channel closed")
	wg.Wait()
	fmt.Println("WaitGroup finished")
	close(results)
	fmt.Println("Results channel closed")
	fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")

	for result := range results {
		fmt.Println("Result :", result)

		if result.FileIdsCount != 1 {
			fmt.Println("File count is not 1, sleeping...")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			countDownTimer(timeout)
			continue
		}

		sheetID := result.FileIds[0]
		cost := ShouldBeSentToCost(sheetID)
		if cost == 0 {
			fmt.Println("Cost is 0, sleeping...")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			countDownTimer(timeout)
			continue
		}

		fmt.Println("Cost: ", cost)

		costSheetID, err := CreateCostSheet(sheetID, result.ParentFolderId, cost)
		if err != nil {
			fmt.Println("Error creating cost sheet")
			fmt.Println(err)
			panic(err)
		}

		fmt.Println("Cost Sheet ID: ", costSheetID)
		fmt.Println("Parsing the sheet")
		fmt.Println(surpriceURLGetCost)

		sheetUrl := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=0", costSheetID)
		fmt.Println("Sheet URL: ", sheetUrl)
		priceResp, err := http.Post(surpriceURLGetCost, "application/json", strings.NewReader(fmt.Sprintf(`{"url": "%s"}`, sheetUrl)))
		if err != nil {
			fmt.Println("Error getting cost from Surprice")
			fmt.Println(err)
			panic(err)
		}
		
		var surpriceResponse SurpriceResponse
		body, err := io.ReadAll(priceResp.Body)
		if err != nil {
			fmt.Println("Error reading response body")
			panic(err)
		}

		if err := json.Unmarshal(body, &surpriceResponse); err != nil {
			fmt.Println("Error unmarshalling response body")
			countDownTimer(timeout)
			continue
		}

		fmt.Println("Surprice Response",modules.PrettyPrint(surpriceResponse))

		http.Post(surpriceURLUpdateCost, "application/json", strings.NewReader(fmt.Sprintf(`{"cost": %d, "sheetId": "%s"}`, cost, costSheetID)))

		fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
		countDownTimer(timeout)
		
	}

}
