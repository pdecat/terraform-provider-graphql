package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func queryExecute(ctx context.Context, d *schema.ResourceData, m interface{}, querySource, variableSource string) (*GqlQueryResponse, []byte, error) {
	query := d.Get(querySource).(string)
	inputVariables := d.Get(variableSource).(map[string]interface{})
	apiURL := m.(*graphqlProviderConfig).GQLServerUrl
	headers := m.(*graphqlProviderConfig).RequestHeaders
	authenticatedHeaders := m.(*graphqlProviderConfig).RequestAuthenticatedHeaders

	var queryBodyBuffer bytes.Buffer

	queryObj := GqlQuery{
		Query:     query,
		Variables: make(map[string]interface{}), // Create an empty map to be populated below
	}

	// Populate GqlQuery variables
	for k, v := range inputVariables {
		// Convert any json string inputs to a struct for complex GraphQL inputs
		js, isJS := isJSON(v)

		if isJS {
			queryObj.Variables[k] = js
		} else {
			// If the input is just a simple string/not JSON
			queryObj.Variables[k] = v
		}
	}

	if err := json.NewEncoder(&queryBodyBuffer).Encode(queryObj); err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &queryBodyBuffer)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	for key, value := range headers {
		req.Header.Set(key, value.(string))
	}
	for key, value := range authenticatedHeaders {
		req.Header.Set(key, value.(string))
	}

	client := &http.Client{}
	if logging.IsDebugOrHigher() {
		log.Printf("[DEBUG] Enabling HTTP requests/responses tracing")
		client.Transport = logging.NewTransport("GraphQL", http.DefaultTransport)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var gqlResponse GqlQueryResponse
	if err := json.Unmarshal(body, &gqlResponse); err != nil {
		return nil, nil, fmt.Errorf("unable to parse graphql server response: %v ---> %s", err, string(body))
	}

	return &gqlResponse, body, nil
}

// isJSON will check if s can be interpreted as valid JSON, and return an unmarshalled struct representing the JSON if it can.
func isJSON(s interface{}) (interface{}, bool) {
	var js interface{}
	err := json.Unmarshal([]byte(s.(string)), &js)
	if err != nil {
		return nil, false
	}
	return js, true
}
