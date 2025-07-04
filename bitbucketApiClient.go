package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/sethgrid/pester"
)

// BitbucketAPIClient communicates with the Bitbucket api
type BitbucketAPIClient interface {
	SetBuildStatus(accessToken, repoFullname, gitRevision, status, buildVersion, releaseName, releaseAction string) error
}

type bitbucketAPIClientImpl struct {
}

func newBitbucketAPIClient() BitbucketAPIClient {
	return &bitbucketAPIClientImpl{}
}

type buildStatusRequestBody struct {
	State       string `json:"state"`
	Key         string `json:"key"`
	Name        string `json:"name,omitempty"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// SetBuildStatus sets the build status for a specific revision
func (gh *bitbucketAPIClientImpl) SetBuildStatus(accessToken, repoFullname, gitRevision, status, buildVersion, releaseName, releaseAction string) (err error) {

	// https://confluence.atlassian.com/bitbucket/buildstatus-resource-779295267.html
	// ziplinee status: succeeded|failed|pending
	// bitbucket stat: INPROGRESS|SUCCESSFUL|FAILED|STOPPED

	state := "SUCCESSFUL"
	switch status {
	case "succeeded":
		state = "SUCCESSFUL"

	case "failed":
		state = "FAILED"

	case "pending":
		state = "INPROGRESS"
	}

	logsURL := fmt.Sprintf(
		"%vpipelines/%v/%v/builds/%v/logs",
		*ciBaseURL,
		*gitRepoSource,
		repoFullname,
		*ziplineeBuildID,
	)

	// set description depending on status
	description := fmt.Sprintf("Build version %v %v.", *ziplineeBuildVersion, status)
	if releaseName != "" {
		description = fmt.Sprintf("Release %v to %v %v.", *ziplineeBuildVersion, releaseName, status)
		if releaseAction != "" {
			description = fmt.Sprintf("Release %v to %v with %v %v.", *ziplineeBuildVersion, releaseName, releaseAction, status)
		}
	}

	params := buildStatusRequestBody{
		State:       state,
		Key:         "ziplinee",
		Name:        "Ziplinee",
		URL:         logsURL,
		Description: description,
	}

	// {
	// 	"state": "<INPROGRESS|SUCCESSFUL|FAILED>",
	// 	"key": "<build-key>",
	// 	"name": "<build-name>",
	// 	"url": "<build-url>",
	// 	"description": "<build-description>"
	// }

	log.Printf("Setting logs url %v", params.URL)

	_, err = callBitbucketAPI("POST", fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%v/commit/%v/statuses/build", repoFullname, gitRevision), params, "Bearer", accessToken)

	return
}

func callBitbucketAPI(method, url string, params interface{}, authorizationType, token string) (body []byte, err error) {

	// convert params to json if they're present
	var requestBody io.Reader
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return body, err
		}
		requestBody = bytes.NewReader(data)
	}

	// create client, in order to add headers
	client := pester.New()
	client.MaxRetries = 3
	client.Backoff = pester.ExponentialJitterBackoff
	client.KeepLog = true
	request, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return
	}

	// add headers
	request.Header.Add("Authorization", fmt.Sprintf("%v %v", authorizationType, token))
	request.Header.Add("Content-Type", "application/json")

	// perform actual request
	response, err := client.Do(request)
	if err != nil {
		return
	}

	defer response.Body.Close()

	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	// unmarshal json body
	var b interface{}
	err = json.Unmarshal(body, &b)
	if err != nil {
		log.Printf("Deserializing response for '%v' Bitbucket api call failed. Body: %v. Error: %v", url, string(body), err)
		return
	}

	log.Printf("Received successful response for '%v' Bitbucket api call with status code %v", url, response.StatusCode)

	return
}
