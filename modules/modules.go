package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/mwalkersigma/drive-parser/models"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var driveService *drive.Service
var surpriceURLSuspendSheet string

func DaysOld(startDate time.Time, endDate time.Time) int {
	// Calculate the numbers of days rounded down to the nearest date
	hours := endDate.Sub(startDate).Hours()
	days := hours / 24
	math.Trunc(days)
	return int(days)
}

func init() {
	err := godotenv.Load()
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	ds, dsErr := drive.NewService(ctx, option.WithCredentialsFile("./cert.json"))
	if dsErr != nil {
		fmt.Println("Error creating new service")
		panic(dsErr)
	}
	driveService = ds
	surpriceURLSuspendSheet = fmt.Sprintf("%s/api/v1/costSheet/status", os.Getenv("BASE_URL"))
}

func PrettyPrint(i interface{}) string {
	s, err := json.MarshalIndent(i, "", "\t")
	if err != nil {
		return "Error"
	}
	return string(s)
}

type FileDetails struct {
	Name string
	Id   string
}

type WorkerResult struct {
	FileDetails    []FileDetails
	ParentFolderId string
	FileIdsCount   int
	// need a date field for creation
	CreatedAt time.Time
	Age       int
}

func Worker(jobs <-chan string, results chan<- WorkerResult) {
	fmt.Println("Worker started")
	for j := range jobs {
		innerFiles, err := driveService.
			Files.
			List().
			Fields("files(id, name, createdTime)").
			Q(fmt.Sprintf("'%s' in parents and mimeType != 'application/vnd.google-apps.folder' ", j)).
			Do()
		if err != nil {
			fmt.Println("Error getting files from folder")
			panic(err)
		}
		var fileIds []FileDetails
		var CreatedTime time.Time
		var age int
		endDate := time.Now()
		for _, file := range innerFiles.Files {

			fmt.Println("File Create Date: ", file.CreatedTime)
			CreatedTime, err = time.Parse(time.RFC3339, file.CreatedTime)
			if err != nil {
				fmt.Println("Error parsing time")
				panic(err)
			}
			age = DaysOld(CreatedTime, endDate)
			fmt.Println("File: ", file.Name, " ID: ", file.Id, "Created: ", CreatedTime, "Age: ", age)
			fileDetails := FileDetails{Name: file.Name, Id: file.Id}
			fileIds = append(fileIds, fileDetails)
		}

		results <- WorkerResult{FileDetails: fileIds, FileIdsCount: len(innerFiles.Files), ParentFolderId: j, CreatedAt: CreatedTime, Age: age}
	}
	fmt.Println("Worker finished")
}

func SetupWorkers(workerCount int, jobCount int) (chan string, chan WorkerResult, *sync.WaitGroup) {
	jobs := make(chan string, jobCount)
	results := make(chan WorkerResult, jobCount)
	wg := sync.WaitGroup{}
	for w := 1; w <= workerCount; w++ {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			Worker(jobs, results)
		}(w)
	}
	return jobs, results, &wg
}

func MarkSheet(reason string, resolution string) func(string, string) (bool, error) {
	return func(sheetID string, title string) (bool, error) {
		var client = &http.Client{Timeout: 60 * time.Second * 5}
		expectedSuccessResponse := "Sheet has been marked with failure reason"
		fmt.Println("URL: ", surpriceURLSuspendSheet)
		body := fmt.Sprintf(`{"sheetID": "%s", "reason": "%s", "resolution": "%s", "title": "%s"}`, sheetID, reason, resolution, title)
		fmt.Println("Body: ", body)
		resp, err := client.Post(surpriceURLSuspendSheet,
			"application/json",
			strings.NewReader(body))
		if err != nil {
			fmt.Println("Error calling Drive Parser to suspend sheet")
			fmt.Println(err)
			return false, err
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				fmt.Println("Error closing response body")
				fmt.Println(err)
			}
		}(resp.Body)

		var target models.DriveStatusResponse
		err = json.NewDecoder(resp.Body).Decode(&target)
		if err != nil {
			fmt.Println("Error decoding response")
			fmt.Println(err)
			return false, err
		}
		fmt.Println("Response: ", target.Message)
		correctResponse := target.Message == expectedSuccessResponse
		return correctResponse, nil
	}
}
func MarkSheetSuspended(sheetID string, title string) (bool, error) {
	reason := "Sheet has not had cost put in for 60 or more days and is suspended in Insightly"
	resolution := "Please communicate with the Opportunity Owner to determine if the opportunity is still active. If the opportunity is still active, please update the sheet with the correct cost."
	return MarkSheet(reason, resolution)(sheetID, title)
}
func MarkSheetForgotten(sheetID string, title string) (bool, error) {
	reason := "Sheet is currently in OPEN status and has not been updated in 60 or more days"
	resolution := "Please communicate with the Opportunity Owner to determine if the opportunity is still active. If the opportunity is still active, please update the sheet with the correct cost."
	return MarkSheet(reason, resolution)(sheetID, title)
}

func IsMarked(failureReason string) func(string) (bool, error) {
	return func(SheetID string) (bool, error) {
		var client = &http.Client{Timeout: 60 * time.Second * 5}
		resp, err := client.Get(surpriceURLSuspendSheet + fmt.Sprintf("/%s", SheetID))
		if err != nil {
			fmt.Println("Error calling Drive Parser")
			fmt.Println(err)
			return false, err
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				fmt.Println("Error closing response body")
				fmt.Println(err)
			}
		}(resp.Body)

		var target models.DriveStatusResponse
		err = json.NewDecoder(resp.Body).Decode(&target)
		if err != nil {
			fmt.Println("Error decoding response")
			fmt.Println(err)
			return false, err
		}
		receivedReason := target.Data.SheetFailureReason
		if failureReason == receivedReason && !target.Data.IsReviewed {
			return true, nil
		}
		if target.Data.SheetFailureReason != "" {
			fmt.Println("Failure Reason did not match expected")
			fmt.Println("Expected Reason: ", failureReason)
			fmt.Println("Received Reason: ", receivedReason)
		}
		return false, nil
	}
}
func IsMarkedSuspended(SheetID string) (bool, error) {
	return IsMarked("Sheet has not had cost put in for 60 or more days and is suspended in Insightly")(SheetID)
}
func IsMarkedForgotten(SheetID string) (bool, error) {
	return IsMarked("Sheet is currently in OPEN status and has not been updated in 60 or more days")(SheetID)
}
