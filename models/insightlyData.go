package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const getOpportunityURL = "https://api.insightly.com/v3.1/Opportunities/"

type AbandonedOppType struct {
	State string
	name  string
}

// Opp ID:  40784214
//Opp State:  ABANDONED
//Opp Pipeline ID:  1055391
//Opp Stage ID:  4380676

var abandonedOppTypes = []AbandonedOppType{
	{State: "ABANDONED", name: "Abandoned"},
	{State: "LOST", name: "Lost"},
}
var suspendedOppTypes = []AbandonedOppType{
	{State: "SUSPENDED", name: "Suspended"},
}
var WonOppTypes = []AbandonedOppType{
	{State: "WON", name: "Won"},
}
var OpenOppTypes = []AbandonedOppType{
	{State: "OPEN", name: "Open"},
}

type CustomField struct {
	FieldName     string      `json:"FIELD_NAME"`
	FieldValue    interface{} `json:"FIELD_VALUE"`
	CustomFieldId string      `json:"CUSTOM_FIELD_ID"`
}

type Link struct {
	Details        any    `json:"DETAILS"`
	Role           any    `json:"ROLE"`
	LinkId         int    `json:"LINK_ID"`
	ObjectName     string `json:"OBJECT_NAME"`
	ObjectId       int    `json:"OBJECT_ID"`
	LinkObjectName string `json:"LINK_OBJECT_NAME"`
	LinkObjectId   int    `json:"LINK_OBJECT_ID"`
}

type InsightlyData struct {
	OpportunityId      int           `json:"OPPORTUNITY_ID"`
	OpportunityName    string        `json:"OPPORTUNITY_NAME"`
	OpportunityDetails string        `json:"OPPORTUNITY_DETAILS"`
	OpportunityState   string        `json:"OPPORTUNITY_STATE"`
	ResponsibleUserId  int           `json:"RESPONSIBLE_USER_ID"`
	CategoryId         int           `json:"CATEGORY_ID"`
	ImageUrl           any           `json:"IMAGE_URL"`
	ActualCloseDate    string        `json:"ACTUAL_CLOSE_DATE"`
	DateCreatedUtc     string        `json:"DATE_CREATED_UTC"`
	DateUpdatedUtc     string        `json:"DATE_UPDATED_UTC"`
	PipelineId         int           `json:"PIPELINE_ID"`
	StageId            int           `json:"STAGE_ID"`
	CustomFields       []CustomField `json:"CUSTOMFIELDS"`
	Links              []Link        `json:"LINKS"`
}

func (i *InsightlyData) GetOpportunity(OpportunityId string) (string, error) {
	myClient := &http.Client{Timeout: 60 * time.Second}
	if os.Getenv("INSIGHTLY_API_KEY") == "" {
		return "No API Key found", fmt.Errorf("No API Key found")
	}
	AuthorizationHeader := "Basic " + os.Getenv("INSIGHTLY_API_KEY")
	// Call the endpoint URL with an Authorization header
	req, err := http.NewRequest("GET", getOpportunityURL+OpportunityId, nil)
	if err != nil {
		return err.Error(), err
	}
	req.Header.Add("Authorization", AuthorizationHeader)
	resp, err := myClient.Do(req)
	if err != nil {
		return err.Error(), err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing body")
		}
	}(resp.Body)

	// Decode the response into the InsightlyData struct
	err = json.NewDecoder(resp.Body).Decode(&i)
	if err != nil {
		fmt.Println("Error decoding response")
		fmt.Println("Expected an InsightlyData struct")
		fmt.Println("Got: ", resp.Body)
		return err.Error(), err
	}
	return "Successfully retrieved opp", nil
}

func (i *InsightlyData) IsAbandoned() bool {
	for _, v := range abandonedOppTypes {
		if i.OpportunityState == v.State {
			return true
		}
	}
	return false
}

func (i *InsightlyData) IsSuspended() bool {
	for _, v := range suspendedOppTypes {
		if i.OpportunityState == v.State {
			return true
		}
	}
	return false
}

func (i *InsightlyData) IsWon() bool {
	for _, v := range WonOppTypes {
		if i.OpportunityState == v.State {
			return true
		}
	}
	return false
}

func (i *InsightlyData) IsOpen() bool {
	for _, v := range OpenOppTypes {
		if i.OpportunityState == v.State {
			return true
		}
	}
	return false
}
