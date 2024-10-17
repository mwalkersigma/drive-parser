package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/mwalkersigma/drive-parser/models"
	"github.com/mwalkersigma/drive-parser/modules"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	_ "google.golang.org/api/sheets/v4"
	sheets "google.golang.org/api/sheets/v4"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var ParentFolderID = `1nhi_QzxkU2maCP5rHG_C9MtTtlY3qDbL`
var retroCostingTemplateID = `1ZLO39C95sDUWPsKfGORIGuw8Ep-oJ5VJ2HCce0i2NM4`
var procurementFolderID = `1TeXMYU9jzWZyna7zB8jngeirvhJosvdO`
var winsFolderId string
var lossesFolderId string
var driveService *drive.Service
var sheetsService *sheets.Service

var surpriceURLUpdateCost, winsFolderName string

var timeout = 1
var rateLimitSleep = 0
var config models.ConfigJson
var start time.Time
var timeSleepingGettingCost = 0
var client = &http.Client{Timeout: 60 * time.Second * 5}

func countDownTimer(duration int) {
	for i := duration; i > 0; i-- {
		// print on the same line
		fmt.Printf("\rSleeping for %d seconds ", i)
		time.Sleep(time.Second)
	}
}

func CallDriveParser(body string, target interface{}) error {
	resp, err := client.Post(surpriceURLUpdateCost, "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Println("Error calling Drive Parser")
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body")
			fmt.Println(err)
		}
	}(resp.Body)

	return json.NewDecoder(resp.Body).Decode(target)
}

func getFolderId(ds *drive.Service, folderName string) (string, error) {
	fmt.Println(fmt.Sprintf("Getting %s folder", folderName))
	var folders []*drive.File
	files, err := ds.Files.List().Q(fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder'", ParentFolderID)).Do()
	if err != nil {
		fmt.Println(fmt.Sprintf("Error getting %s folder", folderName))
		fmt.Println(err)
		return "", err
	}
	folders = append(folders, files.Files...)
	if files.NextPageToken != "" {
		for files.NextPageToken != "" {
			fmt.Println("Next page token found")
			files, err = ds.Files.List().
				Fields("files(id, name), nextPageToken").
				Q(fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder'", ParentFolderID)).
				PageToken(files.NextPageToken).Do()
			if err != nil {
				fmt.Println("Error getting files from folder")
				panic(err)
			}
			folders = append(folders, files.Files...)
		}
	}
	if len(files.Files) < 1 {
		return "", fmt.Errorf("unable to retrieve folders from google")
	}
	for _, file := range files.Files {
		if file.Name == folderName {
			return file.Id, nil
		}
	}

	fmt.Println(fmt.Sprintf(" %s Not Found.", folderName))

	// if we get here, we didn't find the folder
	createFileCall, err := ds.Files.Create(&drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{ParentFolderID},
	}).Do()

	if err != nil {
		fmt.Println(fmt.Sprintf("Error creating %s folder", folderName))
		fmt.Println(err)
		return "", err
	}
	fmt.Println("Folder created successfully")
	return createFileCall.Id, nil
}

func init() {
	start = time.Now()
	err := godotenv.Load()
	if err != nil {
		panic(err)
	}
	err = config.GetConfig()
	if err != nil {
		fmt.Println("Error getting config")
		fmt.Println(err)
		panic(err)
	}
	timeout = config.SleepTimeOut

	winsFolderName = fmt.Sprintf("%s Surplus Procurement Wins", time.Now().Format("2006"))
	fmt.Println("Wins Folder Name: ", winsFolderName)

	lossFolderName := fmt.Sprintf("Surplus Procurement Lost")

	surpriceURLUpdateCost = fmt.Sprintf("%s/api/v1/costSheet/upload", os.Getenv("BASE_URL"))
	fmt.Println("Surprice URL: ", surpriceURLUpdateCost)
	ctx := context.Background()
	ds, driveErr := drive.NewService(ctx, option.WithCredentialsFile("./cert.json"))
	if driveErr != nil {
		fmt.Println("Error creating new service")
		panic(driveErr)
	}
	driveService = ds

	ss, sheetsErr := sheets.NewService(ctx, option.WithCredentialsFile("./SheetCert.json"))
	if sheetsErr != nil {
		fmt.Println("Error creating new service")
		panic(sheetsErr)
	}
	sheetsService = ss

	winsFolderId, err = getFolderId(driveService, winsFolderName)
	fmt.Println("Wins Folder ID: ", winsFolderId)
	if err != nil {
		fmt.Println("Error getting wins folder")
		fmt.Println(err)
		panic(err)
	}

	lossesFolderId, err = getFolderId(driveService, lossFolderName)
	fmt.Println("Losses Folder ID: ", lossesFolderId)

	if err != nil {
		fmt.Println("Error getting lost folder")
		fmt.Println(err)
		panic(err)
	}

}

func decideSheet(result modules.WorkerResult) (sheetId string, hasCostSheet bool, sheetFound bool, name string) {
	var resultFileDetails modules.FileDetails
	for _, fileDetails := range result.FileDetails {
		if strings.Contains(fileDetails.Name, "Cost Sheet") {
			fmt.Println("Cost Sheet Found : ", fileDetails.Name)
			return fileDetails.Id, true, true, fileDetails.Name
		}

		if len(strings.Split(fileDetails.Name, "-")) == 3 {
			resultFileDetails = fileDetails
			sheetId = fileDetails.Id
			sheetFound = true
			hasCostSheet = false
			name = fileDetails.Name
		}
	}

	if sheetFound {
		fmt.Println("No Cost Sheet was found. \n Using the pricing sheet : ", resultFileDetails.Name)
		return sheetId, hasCostSheet, sheetFound, name
	}

	return "", false, false, ""

}

func ShouldBeSentToCost(sheetID string) (cost int, hasCost bool, err error) {
	var sheetAcceptedOffer = "T3"
	sheetRange := fmt.Sprintf("Final Offer!%s", sheetAcceptedOffer)
	fmt.Println("Sheet Range: ", sheetRange)
	callStartTime := time.Now()
	defer func() {
		timeTaken := time.Since(callStartTime)
		timeSleepingGettingCost += int(timeTaken.Seconds())
		fmt.Println("Time taken to get cost: ", timeTaken)
	}()
	resp, err := sheetsService.Spreadsheets.Values.Get(sheetID, sheetRange).Do()
	if err != nil {
		fmt.Println("Error getting sheet")
		fmt.Println(err)
		var maxRetries = 3
		var retryTimeout = 1
		if strings.Contains(err.Error(), "googleapi: Error 429") {
			retryTimeout = 60
			rateLimitSleep++
			fmt.Println("Google API Limit Reached Waiting for 60 seconds")
		} else {
			fmt.Println("Retrying Due to Google Err")
		}
		for i := 0; i < maxRetries; i++ {
			fmt.Println("Retry Attempt: ", i+1)
			time.Sleep(time.Duration(retryTimeout) * time.Millisecond)
			resp, err = sheetsService.Spreadsheets.Values.Get(sheetID, sheetRange).Do()
			if err != nil {
				if strings.Contains(err.Error(), "googleapi: Error 429") {
					rateLimitSleep++
					fmt.Println("Google API Limit Reached Waiting for 60 seconds")
					retryTimeout = 60
				} else {
					fmt.Println("Retrying Due to Google Err")
					retryTimeout = retryTimeout * 2
				}
				continue
			}
			break
		}
		if err != nil {
			fmt.Println("Retries exhausted")
			fmt.Println("Error getting sheet")
			fmt.Println(err)
			return 0, false, err
		}
	}
	fmt.Println("Response: ", modules.PrettyPrint(resp))
	if len(resp.Values) < 1 {
		fmt.Println("No data found in row")
		return 0, false, nil
	}
	if len(resp.Values[0]) < 1 {
		fmt.Println("No data found in cell")
		return 0, false, nil
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

	return cost, true, nil
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

func moveToFolder(folderID string, destFolderId string) (bool, error) {
	_, err := driveService.Files.Update(folderID, &drive.File{}).AddParents(destFolderId).RemoveParents(procurementFolderID).Do()
	if err != nil {
		fmt.Println("Error moving folder")
		fmt.Println(err)
		return false, err

	}
	fmt.Println("Folder moved successfully")
	return true, nil
}

func moveToWinsFolder(folderId string) (bool, error) {
	return moveToFolder(folderId, winsFolderId)
}
func moveToLossesFolder(folderId string) (bool, error) {
	return moveToFolder(folderId, lossesFolderId)
}

func handleNoCostSheet(sheetID string, result modules.WorkerResult, sheetName string) (costSheetId string, shouldSkip bool, needsTimeout bool, err error) {
	isSuspended, err := modules.IsMarkedSuspended(sheetID)
	if err != nil {
		fmt.Println("Error checking if sheet is marked suspended")
		fmt.Println(err)
		return "", true, false, err
	}
	if isSuspended {
		fmt.Println("Sheet is marked suspended")
		return "", true, false, nil
	}
	isForgotten, err := modules.IsMarkedForgotten(sheetID)
	if err != nil {
		fmt.Println("Error checking if sheet is marked suspended")
		fmt.Println(err)
		return "", true, false, err
	}
	if isForgotten {
		fmt.Println("Sheet is marked forgotten")
		return "", true, false, nil
	}
	cost, hasCost, err := ShouldBeSentToCost(sheetID)
	if err != nil {
		fmt.Println("Error getting cost")
		fmt.Println(err)
		return "", true, true, err
	}
	if hasCost {
		createdSheetID, costSheetName, err := CreateCostSheet(sheetID, result.ParentFolderId, cost)
		if err != nil {
			fmt.Println("Error creating cost sheet")
			fmt.Println(err)
			return "", true, true, err
		}
		fmt.Println("Cost Sheet ID: ", createdSheetID)
		fmt.Println("Cost Sheet Name: ", costSheetName)
		fmt.Println("Cost Sheet created successfully")
		return createdSheetID, false, true, nil
	}

	fmt.Println("No cost found")
	fmt.Println("Sheet Age: ", result.Age)
	if result.Age >= 60 {
		fmt.Println("Sheet is older than 60 days -> Checking Insightly to see if it is lost")
		var oppId = strings.Split(sheetName, "-")[2]
		if oppId == "" {
			fmt.Println("No opportunity ID found")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			return "", true, true, nil
		}
		fmt.Println("Opportunity ID: ", oppId)
		var i models.InsightlyData
		message, err := i.GetOpportunity(oppId)
		if err != nil {
			fmt.Println("Error getting opportunity. Opportunity may not exist.")
			fmt.Println(err)
			if strings.Contains(err.Error(), "json: cannot unmarshal") {
				fmt.Println("Opportunity not found")
				folderWasMoved, err := moveToLossesFolder(result.ParentFolderId)
				if err != nil {
					fmt.Println("Error moving folder")
					fmt.Println(err)
					return "", true, true, err
				}
				if folderWasMoved {
					fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
					return "", true, true, nil
				}
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				return "", true, true, nil
			}
			return "", true, true, err
		}
		fmt.Println(message)
		if i.IsAbandoned() {
			fmt.Println("Opportunity is abandoned")
			folderWasMoved, err := moveToLossesFolder(result.ParentFolderId)
			if err != nil {
				fmt.Println("Error moving folder")
				fmt.Println(err)
				return "", true, true, err
			}
			if folderWasMoved {
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				return "", true, true, nil
			}
		}
		if i.IsWon() {
			fmt.Println("Opportunity is won")
			folderWasMoved, err := moveToWinsFolder(result.ParentFolderId)
			if err != nil {
				fmt.Println("Error moving folder")
				fmt.Println(err)
				return "", true, true, err
			}
			if folderWasMoved {
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				return "", true, true, nil
			}
		}
		if i.IsSuspended() {
			fmt.Println("Opportunity is suspended")
			marked, err := modules.MarkSheetSuspended(sheetID, sheetName)
			if err != nil {
				fmt.Println("Error marking sheet suspended")
				fmt.Println(err)
				return "", true, true, err
			}
			if marked {
				fmt.Println("Sheet marked as suspended")
			} else {
				fmt.Println("Sheet not marked as suspended")
			}
			return "", true, true, nil
		}
		if i.IsOpen() {
			fmt.Println("Sheet is older than 60 days -> Marking as forgotten")
			marked, err := modules.MarkSheetForgotten(sheetID, sheetName)
			if err != nil {
				fmt.Println("Error marking sheet as forgotten")
				fmt.Println(err)
				return "", true, true, err
			}
			if marked {
				fmt.Println("Sheet marked as forgotten")
			} else {
				fmt.Println("Sheet not marked as forgotten")
			}
			return "", true, true, nil
		}
		fmt.Println("Opportunity is not lost or suspended")
		fmt.Println("Opp ID: ", oppId)
		fmt.Println("Opp State: ", i.OpportunityState)
	}
	return "", true, true, err
}

func main() {
	elapsedTimeWaitingForAPI := 0
	processedFiles := 0
	sleeplessFiles := 0
	callsToDriveParser := 0
	posGenerated := 0

	var fileList []*drive.File
	files, err := driveService.
		Files.
		List().
		Fields("files(id, name), nextPageToken").
		Q(fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder'", procurementFolderID)).
		Do()
	if err != nil {
		fmt.Println("Error fetching files")
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

	fmt.Printf("Found %d files", len(fileList))
	jobs, results, wg := modules.SetupWorkers(10, len(fileList))

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
		fmt.Println()
		processedFiles++
		var costSheetID string
		sheetID, hasCostSheet, sheetFound, chosenSheetName := decideSheet(result)
		if !sheetFound {
			fmt.Println("Sheet not found")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			countDownTimer(timeout)
			continue
		}

		if hasCostSheet {
			costSheetID = sheetID
		} else {
			csID, shouldSkip, needsTimeout, handleCostErr := handleNoCostSheet(sheetID, result, chosenSheetName)
			if handleCostErr != nil {
				fmt.Println("Error handling no cost sheet")
				fmt.Println(handleCostErr)
				countDownTimer(timeout)
				continue
			}
			if shouldSkip {
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				if needsTimeout {
					countDownTimer(timeout)
				} else {
					sleeplessFiles++
				}
				continue
			}
			if csID == "" {
				fmt.Println("No cost sheet ID found")
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
				countDownTimer(timeout)
				continue
			}
			costSheetID = csID
		}

		sheetUrl := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=0", costSheetID)
		fmt.Println("Calling the Drive Parser with Sheet URL -> : ", sheetUrl)
		jsonData := models.DriveParserResponse{}
		startApiCall := time.Now()
		callsToDriveParser++
		err := CallDriveParser(fmt.Sprintf(`{"url": "%s"}`, sheetUrl), &jsonData)
		if err != nil {
			fmt.Println("Error calling Drive Parser")
			fmt.Println(err)
			countDownTimer(timeout)
			continue
		}
		elapsedTimeWaitingForAPI += int(time.Since(startApiCall).Seconds())
		fmt.Println("Response Message: ", jsonData.Message)
		if !jsonData.Error {

			moved, err := moveToWinsFolder(result.ParentFolderId)
			if err != nil {
				fmt.Println("Error moving folder")
				fmt.Println(err)
				countDownTimer(timeout)
				continue
			}

			if moved {
				fmt.Println("Folder moved successfully")
			}
			if jsonData.Message == "Sheet has already been processed" || jsonData.Message == "PO Already Exists" {
				fmt.Println("Sheet has already been processed")
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			} else if jsonData.Message == "PO Created Successfully" {
				posGenerated++
				fmt.Println("Sheet was successfully processed and sent to sku vault")
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			} else {
				fmt.Println("No Explicit handler for : ", jsonData.Message)
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			}
			countDownTimer(timeout)
			continue

		}
		trimmedMessage := strings.TrimSpace(jsonData.Message)
		switch trimmedMessage {
		case "Supplier Name could not be determined":
			fmt.Println("Supplier Name could not be determined")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
		case "Some items were skipped because they had no SKU or Quantity":
			fmt.Println("Items: ", jsonData.Data)
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
		case "Error updating sheet: Request failed with status code 502":
			fmt.Println("Retrying Sheet")
			for retries := 0; retries < 2; retries++ {
				err := CallDriveParser(fmt.Sprintf(`{"url": "%s"}`, sheetUrl), &jsonData)
				if err != nil {
					fmt.Println("Error calling Drive Parser")
					fmt.Println(err)
					continue
				}
				if jsonData.Message == "Error updating sheet: Request failed with status code 502" {
					fmt.Println("Retrying")
					continue
				}
				break
			}
			fmt.Println("Unable to process sheet after retries")
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
		case "PO Already Exists":
			fmt.Println("PO Already Exists")
			moved, err := moveToWinsFolder(result.ParentFolderId)
			if err != nil {
				fmt.Println("Error moving folder")
				fmt.Println(err)
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			}
			if moved {
				fmt.Println("Folder moved successfully")
				fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
			}
		default:
			fmt.Println("No Explicit handler for : ", trimmedMessage)
			fmt.Println("-=-=-=-=-=-=-=-=-=-=-=-")
		}
		countDownTimer(timeout)
		continue
	}
	fmt.Println()
	fmt.Println("All files processed")

	end := time.Now()
	elapsed := end.Sub(start)

	fmt.Println(fmt.Sprintf("Processed %d Files", processedFiles))
	fmt.Println(fmt.Sprintf("Total Execution time: %s", elapsed))
	fmt.Println(fmt.Sprintf("Total POs Generated: %d", posGenerated))
	secondsWaitingForRateLimit := rateLimitSleep * 60
	fmt.Println(fmt.Sprintf("Total Time Waiting for Rate Limit: %d seconds", secondsWaitingForRateLimit))
	secondsSleeping := ((processedFiles - 2 - sleeplessFiles) * timeout) + secondsWaitingForRateLimit

	durationSleeping := time.Duration(secondsSleeping * int(time.Second))
	percentOfExecutionTime := (durationSleeping.Seconds() / elapsed.Seconds()) * 100
	fmt.Println(fmt.Sprintf("Total time sleeping: %s || %.2f%% Percentage of total execution time ", durationSleeping, percentOfExecutionTime))

	durationWaitingForCost := time.Duration(timeSleepingGettingCost) * time.Second
	percentOfExecutionTime = (durationWaitingForCost.Seconds() / elapsed.Seconds()) * 100
	fmt.Println(fmt.Sprintf("Total time waiting for cost: %s || %.2f%% Percentage of total execution time", durationWaitingForCost, percentOfExecutionTime))

	durationWaitingForApi := time.Duration(elapsedTimeWaitingForAPI) * time.Second
	percentOfExecutionTime = (durationWaitingForApi.Seconds() / elapsed.Seconds()) * 100
	fmt.Println(fmt.Sprintf("Total Calls to Drive Parser API: %d", callsToDriveParser))
	fmt.Println(fmt.Sprintf("Total time waiting for Drive Parser API: %s || %.2f%% Percentage of total execution time", durationWaitingForApi, percentOfExecutionTime))

	localProcessingTime := elapsed - durationWaitingForApi - durationSleeping - durationWaitingForCost
	percentOfExecutionTime = (localProcessingTime.Seconds() / elapsed.Seconds()) * 100
	fmt.Println(fmt.Sprintf("Total time Processing Data locally: %s || %.2f%% Percentage of total execution time", localProcessingTime, percentOfExecutionTime))

	fmt.Println("Sending Statistics to Surtrics")

	stats := models.Statistics{
		Start:                             start,
		End:                               end,
		Completed:                         true,
		TotalFiles:                        len(fileList),
		SkippedFiles:                      sleeplessFiles,
		ProcessedFiles:                    processedFiles,
		CallsToDriveParser:                callsToDriveParser,
		PosGenerated:                      posGenerated,
		TotalExecutionTime:                elapsed.String(),
		TotalTimeWaitingForCost:           durationWaitingForCost.String(),
		TotalTimeSleeping:                 durationSleeping.String(),
		TotalTimeWaitingForDriveParserApi: durationWaitingForApi.String(),
	}
	url := fmt.Sprintf("%s/run", surpriceURLUpdateCost)
	fmt.Println("Sending Stats to: ", url)
	stats.SendStats(url, client)
}
