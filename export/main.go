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
	"os"
	"strings"
	"time"
)

var procurementFolderID = `1TeXMYU9jzWZyna7zB8jngeirvhJosvdO`
var timeout = 1

var driveService *drive.Service
var sheetsService *sheets.Service
var p models.ParsedDrivesJson

func getLink(id string) string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", id)
}

func init() {
	p.GetDrives()
	fmt.Println("Removing Non - Cost Sheets")

	for i := 0; i < len(p.Drives); i++ {
		if !strings.Contains(p.Drives[i], "Cost") {
			p.Drives = append(p.Drives[:i], p.Drives[i+1:]...)
			i--
		}
	}
	fmt.Println("Cost Sheets: ", len(p.Drives))
	// remove duplicates
	for i := 0; i < len(p.Drives); i++ {
		for j := i + 1; j < len(p.Drives); j++ {
			if p.Drives[i] == p.Drives[j] {
				p.Drives = append(p.Drives[:j], p.Drives[j+1:]...)
				j--
			}
		}
	}

	fmt.Println("Drives Acquired after de dupe: ", len(p.Drives))

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
	fmt.Println("Init Complete. Starting Costing Sheet Sku Export to CSV...")
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

	var costSheetsToParse []modules.FileDetails

	for driveFile := range results {
		for _, file := range driveFile.FileDetails {
			if !strings.Contains(file.Name, "Cost") {
				continue
			}
			fmt.Println("Cost Sheet Found: ", file.Name)
			for _, parsedFile := range p.Drives {
				if strings.Contains(parsedFile, file.Name) {
					fmt.Println("Match Found: ", file.Name)
					fmt.Println("File ID: ", file.Id)
					fmt.Println("Parent Folder ID: ", driveFile.ParentFolderId)
					fmt.Println("File Count: ", driveFile.FileIdsCount)
					costSheetsToParse = append(costSheetsToParse, file)
				}
			}
		}
	}
	fmt.Println("Cost Sheets to Parse: ", len(costSheetsToParse))
	csv := "po_number,sku,cost,link\n"
	var retryCount = 0
	const maxRetries = 3
	// get the cost sheet data
	for i := 0; i < len(costSheetsToParse); i++ {
		costSheet := costSheetsToParse[i]
		fmt.Println("Parsing Cost Sheet: ", costSheet.Name)
		costSheetData, err := sheetsService.
			Spreadsheets.Values.
			Get(costSheet.Id, "Offer Template!J:P").
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
		for j := 0; j < len(costSheetData.Values); j++ {
			row := costSheetData.Values[j]
			if row[0] == "" {
				costSheetData.Values = append(costSheetData.Values[:j], costSheetData.Values[j+1:]...)
				j--
			}
		}
		// remove the header row
		costSheetData.Values = costSheetData.Values[1:]
		fmt.Printf("Found %d rows \n ", len(costSheetData.Values))
		poNumber := costSheet.Name
		link := getLink(costSheet.Id)
		for _, row := range costSheetData.Values {
			sku := row[1].(string)
			cost := row[6].(string)
			csv += fmt.Sprintf("%s,%s,%s,%s\n", poNumber, sku, cost, link)
		}
		// We need columns J - P
		// CSV columns: po_number, sku, cost
		// PO comes from the sheet title
		// Sku comes from column J
		// Cost comes from column P

	}
	// write the csv to a file
	outfile, err := os.Create("./export/cost_export.csv")
	if err != nil {
		panic(err)
	}
	defer outfile.Close()

	_, err = outfile.WriteString(csv)
	if err != nil {
		panic(err)
	}
	fmt.Println("CSV written successfully")
	fmt.Println("Program finished")

}
