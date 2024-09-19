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
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var retroCostingTemplateID = `1ZLO39C95sDUWPsKfGORIGuw8Ep-oJ5VJ2HCce0i2NM4`
var procurementFolderID = `1TeXMYU9jzWZyna7zB8jngeirvhJosvdO`

var timeout = 1

// ParsedDrivesJson is the struct for the parsed json file
// it is an array of strings

var surpriceURLGetCost, surpriceURLUpdateCost string
var driveService *drive.Service
var sheetsService *sheets.Service
var p models.ParsedDrivesJson
var status models.Status

func countDownTimer(duration int) {
	for i := duration; i > 0; i-- {
		// print on the same line
		fmt.Printf("\rSleeping for %d seconds ", i)
		time.Sleep(time.Second)
	}
}

func init() {
	p.GetDrives()
	status.GetStatus()
	fmt.Println(p.Drives)
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

// ShouldBeSentToCost
// This function determines if a pricing sheet has a final offer amount.
// if it does, it returns the amount as an integer
// if it does not, it returns 0
func ShouldBeSentToCost(sheetID string) (cost int, err error) {
	var sheetAcceptedOffer string = "T3"
	sheetRange := fmt.Sprintf("Final Offer!%s", sheetAcceptedOffer)
	fmt.Println("Sheet Range: ", sheetRange)
	resp, err := sheetsService.Spreadsheets.Values.Get(sheetID, sheetRange).Do()
	if err != nil {
		fmt.Println("Error getting sheet")
		fmt.Println("Sheet ID: ", sheetID)
		fmt.Println(err)
		return 0, err
	}
	fmt.Println("Response: ", modules.PrettyPrint(resp))
	if len(resp.Values) < 1 {
		fmt.Println("No data found in row")
		return 0, nil
	}
	if len(resp.Values[0]) < 1 {
		fmt.Println("No data found in cell")
		return 0, nil
	}
	fmt.Println("Data: ", resp.Values[0][0])

	currencyStr := resp.Values[0][0].(string)
	currencyStr = strings.Split(currencyStr, "$")[1]
	currencyStr = strings.Replace(currencyStr, ",", "", -1)

	cost, err = strconv.Atoi(currencyStr)
	if err != nil {
		fmt.Println("Error converting currency string to int")
		fmt.Println(err)
		panic(err)
	}

	return cost, nil
}

func CreateCostSheet(sheetID string, parentFolderId string, cost int) (respId string, costSheetName string, err error) {
	costDataRange := "A2:D"
	costDataRange = fmt.Sprintf("Final Offer!%s", costDataRange)

	// Get the title from the sheet
	sheetToCopy, err := sheetsService.Spreadsheets.Get(sheetID).Do()
	if err != nil {
		fmt.Println("Error getting sheet")
		fmt.Println("Sheet ID: ", sheetID)
		fmt.Println(err)
		return "", "", err
	}
	title := sheetToCopy.Properties.Title

	costData, err := sheetsService.Spreadsheets.Values.Get(sheetID, costDataRange).Do()
	if err != nil {
		fmt.Println("Error getting sheet")
		fmt.Println("Sheet ID: ", sheetID)
		fmt.Println(err)
		return "", "", err
	}
	costSheetName = fmt.Sprintf("%s - Cost Sheet - %s", title, time.Now().Format("2006-01-02"))
	resp, err := driveService.Files.Copy(retroCostingTemplateID, &drive.File{
		Name:    costSheetName,
		Parents: []string{parentFolderId},
	}).Do()
	if err != nil {
		fmt.Println("Error copying file")
		fmt.Println(err)
		return "", "", err
	}

	fmt.Println("File copied successfully")
	fmt.Println("File ID: ", resp.Id)
	copyDataRange := "A2:D"
	copyDataRange = fmt.Sprintf("Offer Template!%s", copyDataRange)

	costData.Range = copyDataRange

	update, err := sheetsService.Spreadsheets.Values.Update(resp.Id, copyDataRange, costData).ValueInputOption("RAW").Do()
	if err != nil {
		fmt.Println("Error updating sheet")
		fmt.Println(err)
		return "", "", err
	}
	fmt.Println("Update: ", update)
	fmt.Println("Cost data updated successfully")

	update, err = sheetsService.Spreadsheets.Values.Update(resp.Id, "Offer Template!S3", &sheets.ValueRange{
		Values:         [][]interface{}{{cost}},
		Range:          "Offer Template!S3",
		MajorDimension: "ROWS",
	}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		fmt.Println("Error updating cost on sheet")
		fmt.Println(err)
		return "", "", err
	}
	fmt.Println("Update: ", update)
	fmt.Println("Cost updated successfully")

	return resp.Id, costSheetName, nil
}

func handleInterrupt() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		status.Running = false
		status.Save()
		os.Exit(1)
	}()
}

func handleRunning() {
	status.Running = true
	status.Save()
}

func decideSheet(result modules.WorkerResult) (sheetId string, hasCostSheet bool, sheetFound bool) {

	for _, fileDetails := range result.FileDetails {
		if strings.Contains(fileDetails.Name, "Cost Sheet") {
			return fileDetails.Id, true, true
		}
		if len(strings.Split(fileDetails.Name, "-")) == 3 {
			sheetId = fileDetails.Id
			sheetFound = true
			hasCostSheet = false
		}
	}

	if sheetFound {
		return sheetId, hasCostSheet, sheetFound
	}

	return "", false, false

}

func main() {
	// if the program is running and interrupted with ctrl + c then set the status to not running
	handleInterrupt()

	// if the program is already running then return
	fmt.Println("Status: ", status.Running)
	if status.Running {
		fmt.Println("Already running")
		return
	}
	handleRunning()
	defer func() {
		status.Running = false
		status.Save()
	}()

	fmt.Println("Starting main function")

	var fileList []*drive.File
	defer p.SaveDrives()

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

	fmt.Println("Files Length: ", len(fileList))
	// remove any files from the file list in p.Drives
	for _, storageDrive := range p.Drives {
		// storageDrive = strings.Split(storageDrive, "-")[0]
		for i, file := range fileList {
			if file.Name == storageDrive {
				fmt.Println("Removing file: ", file.Name)
				fileList = append(fileList[:i], fileList[i+1:]...)
			}
		}
	}
	fmt.Println("Files Length: ", len(fileList))
	jobs, results, wg := modules.SetupWorkers(10, len(fileList))
	fmt.Println("Jobs, Results and WaitGroup created successfully")

	for _, file := range fileList {
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

		var costSheetName, costSheetID string
		var cost int
		//todo make sure we are properly handling all cases
		sheetID, hasCostSheet, sheetFound := decideSheet(result)
		if !sheetFound {
			fmt.Println("Sheet not found")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			continue
		}
		if hasCostSheet {
			costSheetID = sheetID
		} else {
			fmt.Println("Sheet found but no cost sheet")
			cost, err = ShouldBeSentToCost(sheetID)
			if err != nil {
				fmt.Println("Error getting cost")
				fmt.Println(err)
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				continue
			}
			if cost == 0 {
				fmt.Println("Cost is 0, sleeping...")
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				countDownTimer(timeout)
				continue
			}
			costSheetID, costSheetName, err = CreateCostSheet(sheetID, result.ParentFolderId, cost)
			if err != nil {
				fmt.Println("This is the error")
				fmt.Println("Error creating cost sheet")
				fmt.Println(err)
				continue
			}
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

		var surpriceResponse models.SurpriceResponse
		err = surpriceResponse.JSON(priceResp)
		if err != nil {
			fmt.Println("Error reading response body")
			fmt.Println(err)
			countDownTimer(timeout)
			continue
		}
		if surpriceResponse.Error {
			fmt.Println("Error getting cost from Surprice")
			fmt.Println("Surprice Response", modules.PrettyPrint(surpriceResponse))
			countDownTimer(timeout)
			continue
		}
		if !surpriceResponse.IsSubmitted {
			fmt.Println("Sheet not submitted")
			fmt.Println("Surprice Response", modules.PrettyPrint(surpriceResponse))

			updateCostResp, err := http.Post(surpriceURLUpdateCost, "application/json", strings.NewReader(fmt.Sprintf(`{"cost": %d, "url": "%s"}`, cost, sheetUrl)))
			if err != nil {
				p.AddCostSheetNotSubmitted(costSheetName)
				fmt.Println("Error updating cost on Surprice")
			}
			// get the status code
			fmt.Println("Update Cost Response: ", updateCostResp.StatusCode)
			if updateCostResp.StatusCode != 200 {
				p.AddCostSheetNotSubmitted(costSheetName)
				fmt.Println("Sheet was not updated successfully Skipping")
			} else {
				fmt.Println("Sheet updated successfully")
				p.AddDrive(costSheetName)
			}
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			countDownTimer(timeout)
		} else {
			fmt.Println("Sheet already submitted")
			p.AddDrive(costSheetName)
			countDownTimer(timeout)
		}

	}

}
