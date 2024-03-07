package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var driveService *drive.Service

func init () {
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

type WorkerResult struct {
	FileIds []string
	ParentFolderId string
	FileIdsCount int
}

func Worker(jobs <-chan string, results chan<- WorkerResult, wg *sync.WaitGroup) {
	fmt.Println("Worker started")
	defer wg.Done()
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
		var fileIds []string
		for _, file := range innerFiles.Files {
			fmt.Println("File: ", file.Name, " ID: ", file.Id)
			fileIds = append(fileIds, file.Id)
			results <- WorkerResult{ FileIds:fileIds, FileIdsCount: len(innerFiles.Files), ParentFolderId: j }
		}
	}
	fmt.Println("Worker finished")
}

func SheetUrl (id string) string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=0", id)
}

func FolderUrl (id string) string {
	return fmt.Sprintf("https://drive.google.com/drive/folders/%s", id)
}

func SetupWorkers ( workerCount int ) ( chan string, chan WorkerResult, *sync.WaitGroup ) {
	jobs := make(chan string, 100)
	results := make(chan WorkerResult, 100)
	wg := sync.WaitGroup{}
	for w := 1; w <= workerCount; w++ {
		wg.Add(1)
		go Worker(jobs, results, &wg)
	}
	return jobs, results, &wg
}