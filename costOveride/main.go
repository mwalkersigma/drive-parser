package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/mwalkersigma/drive-parser/models"
	"github.com/mwalkersigma/drive-parser/modules"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"net/http"
	"os"
	"strings"
	"time"
)

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

var procurementFolderID = `1TeXMYU9jzWZyna7zB8jngeirvhJosvdO`
var timeout = 1

var driveService *drive.Service
var sheetsService *sheets.Service
var p models.ParsedDrivesJson

func getFolders() []*drive.File {
	var fileList []*drive.File

	files, err := driveService.Files.List().
		Fields("files(id, name), nextPageToken").
		Q(fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder'", procurementFolderID)).Do()
	if err != nil {
		fmt.Println("Error getting files from folder")
		panic(err)
	}
	fileList = append(fileList, files.Files...)
	if files.NextPageToken != "" {
		for files.NextPageToken != "" {
			fmt.Println("Next page token found")
			files, err = driveService.Files.List().
				Fields("files(id, name), nextPageToken").
				Q(fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder'", procurementFolderID)).
				PageToken(files.NextPageToken).Do()
			if err != nil {
				fmt.Println("Error getting files from folder")
				panic(err)
			}
			fileList = append(fileList, files.Files...)
		}
	}
	return fileList
}
func countDownTimer(duration int) {
	for i := duration; i > 0; i-- {
		// print on the same line
		fmt.Printf("\rSleeping for %d seconds ", i)
		time.Sleep(time.Second)
	}
}
func init() {
	p.GetDrives()
	err := godotenv.Load()
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	ds, err := drive.NewService(ctx, option.WithCredentialsFile("./cert.json"))
	if err != nil {
		fmt.Println("Error creating new service")
		panic(err)
	}
	driveService = ds
	ss, err := sheets.NewService(ctx, option.WithCredentialsFile("./SheetCert.json"))
	if err != nil {
		fmt.Println("Error creating new service")
		panic(err)
	}

	sheetsService = ss
}

func main() {
	folders := getFolders()

	costSheetsToSubmit := p.CostSheetsNotSubmitted
	var foldersToParse []*drive.File
OUTER:
	for _, costSheet := range costSheetsToSubmit {
		costSheetParentFolder := strings.TrimSpace(strings.Split(costSheet, "- Cost Sheet")[0])
		fmt.Println("Cost Sheet Parent Folder: ", costSheetParentFolder)
		for _, folder := range folders {
			if folder.Name == costSheetParentFolder {
				fmt.Println("Folder Found: ", folder.Name)
				foldersToParse = append(foldersToParse, folder)
				continue OUTER
			}
		}
		fmt.Println("Folder Not Found: ", costSheetParentFolder)

	}
	jobs, results, wg := modules.SetupWorkers(10, len(foldersToParse))
	fmt.Println("Jobs, Results and WaitGroup created successfully")

	for _, folder := range foldersToParse {
		jobs <- folder.Id
	}
	close(jobs)
	wg.Wait()
	fmt.Println("All Jobs Completed")
	fmt.Println("Results Length: ", len(results))
	close(results)

	var costSheetToSubmit []modules.FileDetails
	for result := range results {
		fmt.Println("Result: ", result)
		for _, file := range result.FileDetails {
			if !strings.Contains(file.Name, "Cost") {
				continue
			}
			fmt.Println("Cost Sheet Found: ", file.Name)
			costSheetToSubmit = append(costSheetToSubmit, file)
		}
	}

	fmt.Println("Cost Sheets to Submit: ", len(costSheetToSubmit))
	var retryCount = 0
	const maxRetries = 3
	// get the cost sheet data
	for i := 0; i < len(costSheetToSubmit); i++ {
		costSheet := costSheetToSubmit[i]
		fmt.Println("Parsing Cost Sheet: ", costSheet.Name)
		costSheetData, err := sheetsService.
			Spreadsheets.Values.
			Get(costSheet.Id, "Offer Template!A:P").
			Do()
		if err != nil {
			fmt.Println("Error getting cost sheet data")
			if retryCount < maxRetries {
				i--
				retryCount++
				time.Sleep(time.Duration(timeout*retryCount) * time.Second)
				fmt.Printf("Retrying After a %d timeout ... \n", timeout*retryCount)
				continue
			}
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
		updateUrl := "https://app.skuvault.com/api/products/updateProducts"
		if len(items) < 100 {
			var requestBody SVRequestBody
			requestBody.Items = items
			requestBody.UserToken = os.Getenv("USER_TOKEN")
			requestBody.TenantToken = os.Getenv("TENANT_TOKEN")
			body := requestBody.ToJSON()
			response, err := http.Post(updateUrl, "application/json", strings.NewReader(body))
			if err != nil {
				fmt.Println("Error updating items")
				panic(err)
			}
			fmt.Println("Response: ", response)
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
				requestBody.UserToken = os.Getenv("USER_TOKEN")
				requestBody.TenantToken = os.Getenv("TENANT_TOKEN")
				body := requestBody.ToJSON()
				response, err := http.Post(updateUrl, "application/json", strings.NewReader(body))
				if err != nil {
					fmt.Println("Error updating items")
					panic(err)
				}
				fmt.Println("Response: ", response)
			}
		}

		countDownTimer(timeout)
	}
}
