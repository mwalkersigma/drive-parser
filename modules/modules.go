package modules

import (
	"context"
	"encoding/json"
	"fmt"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"sync"
)

var driveService *drive.Service

func init() {
	ctx := context.Background()
	ds, err := drive.NewService(ctx, option.WithCredentialsFile("./cert.json"))
	if err != nil {
		fmt.Println("Error creating new service")
		panic(err)
	}
	driveService = ds
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
}

func Worker(jobs <-chan string, results chan<- WorkerResult, wg *sync.WaitGroup) {
	fmt.Println("Worker started")
	for j := range jobs {
		innerFiles, err := driveService.
			Files.
			List().
			Q(fmt.Sprintf("'%s' in parents and mimeType != 'application/vnd.google-apps.folder' ", j)).
			Do()
		if err != nil {
			fmt.Println("Error getting files from folder")
			panic(err)
		}
		var fileIds []FileDetails
		for _, file := range innerFiles.Files {
			fmt.Println("File: ", file.Name, " ID: ", file.Id)
			fileDetails := FileDetails{Name: file.Name, Id: file.Id}
			fileIds = append(fileIds, fileDetails)
		}
		results <- WorkerResult{FileDetails: fileIds, FileIdsCount: len(innerFiles.Files), ParentFolderId: j}
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
			Worker(jobs, results, &wg)
		}(w)
	}
	return jobs, results, &wg
}
