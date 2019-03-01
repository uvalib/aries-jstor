package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/publicsuffix"
)

// Version of the service
const version = "1.0.0"

// Config info
var jstorURL string
var jstorProject string
var jstorEmail string
var jstorPass string
var loginCookies []*http.Cookie

// aries is the structure of the response returned by /api/aries/:id
type aries struct {
	Identifiers []string     `json:"identifier,omitempty"`
	ServiceURL  []serviceURL `json:"service_url,omitempty"`
	AdminURL    []string     `json:"administrative_url,omitempty"`
}

type serviceURL struct {
	URL      string `json:"url,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

type jstorResp struct {
	Total  int          `json:"total,omitempty"`
	Assets []jstorAsset `json:"assets,omitempty"`
}
type jstorAsset struct {
	ID               int    `json:"id,omitempty"`
	Filename         string `json:"filename,omitempty"`
	RepresentationID string `json:"representation_id,omitempty"`
}
type jstorResource struct {
	URL  string `json:"url,omitempty"`
	IIIF string `json:"iiif_url,omitempty"`
}

// favHandler is a dummy handler to silence browser API requests that look for /favicon.ico
func favHandler(c *gin.Context) {
}

// versionHandler reports the version of the serivce
func versionHandler(c *gin.Context) {
	c.String(http.StatusOK, "Aries JSTOR version %s", version)
}

// healthCheckHandler reports the health of the serivce
func healthCheckHandler(c *gin.Context) {
	hcMap := make(map[string]string)
	hcMap["AriesJSTOR"] = "true"
	// ping the api with a minimal request to see if it is alive
	url := fmt.Sprintf("%s/projects/%s/assets?with_meta=false&start=0&limit=0", jstorURL, jstorProject)
	_, err := getAPIResponse(url, true)
	if err != nil {
		log.Printf("HealthCheck JSTOR ping failed: %s", err.Error())
		hcMap["JSTOR"] = "false"
	} else {
		hcMap["JSTOR"] = "true"
	}
	c.JSON(http.StatusOK, hcMap)
}

// ariesPing handles requests to the aries endpoint with no params.
func ariesPing(c *gin.Context) {
	c.String(http.StatusOK, "JSTOR Aries API")
}

// ariesLookup will query APTrust for information on the supplied identifer
func ariesLookup(c *gin.Context) {
	// create filters to search by ID and filename
	passedID := c.Param("id")
	var filterTerms []string
	idF := map[string]string{"type": "numeric", "comparison": "eq",
		"value": passedID, "field": "id", "fieldName": "SSID"}
	ifnF := map[string]string{"type": "string", "field": "filename", "fieldName": "Filename",
		"value": fmt.Sprintf("%s*", passedID)}
	filterTerms = append(filterTerms, mapToEncodedString(idF))
	filterTerms = append(filterTerms, mapToEncodedString(ifnF))

	var out aries
	hits := 0
	for _, filter := range filterTerms {
		qp := "with_meta=false&start=0&limit=1&sort=id&dir=DESC&filter="
		URL := fmt.Sprintf("%s/projects/%s/assets?%s[%s]", jstorURL, jstorProject, qp, filter)
		respStr, err := getAPIResponse(URL, true)
		if err != nil {
			unescaped, _ := url.QueryUnescape(filter)
			log.Printf("Query filter %s Failed: %s", unescaped, err.Error())
			continue
		}
		log.Printf("Parsing JSTOR response for %s", passedID)
		var resp jstorResp
		marshallErr := json.Unmarshal([]byte(respStr), &resp)
		if marshallErr != nil {
			log.Printf("Unable to parse response: %s", marshallErr.Error())
			continue
		}
		if resp.Total == 0 {
			continue
		}
		if resp.Total > 1 {
			unescaped, _ := url.QueryUnescape(filter)
			log.Printf("Query filter %s returned more than one hit", unescaped)
			continue
		}
		hits++
		hit := resp.Assets[0]
		out.Identifiers = append(out.Identifiers, strconv.Itoa(hit.ID))
		out.Identifiers = append(out.Identifiers, hit.Filename)
		repURL := fmt.Sprintf("%s/assets/%d/representation/details?_dc=%s", jstorURL, hit.ID, hit.RepresentationID)
		repRespStr, err := getAPIResponse(repURL, true)
		if err == nil {
			var repInfo jstorResource
			marshallErr = json.Unmarshal([]byte(repRespStr), &repInfo)
			if marshallErr == nil {
				if repInfo.URL != "" {
					out.ServiceURL = append(out.ServiceURL, serviceURL{URL: repInfo.URL, Protocol: "image-download"})
				}
				if repInfo.IIIF != "" {
					out.ServiceURL = append(out.ServiceURL, serviceURL{URL: repInfo.IIIF, Protocol: "iiif-presentation"})
				}
			}
		}
		break
	}
	if hits == 0 {
		c.String(http.StatusNotFound, "%s not found", passedID)
	} else {
		c.JSON(http.StatusOK, out)
	}
}

func mapToEncodedString(val map[string]string) string {
	jsonStr, _ := json.Marshal(val)
	t := &url.URL{Path: string(jsonStr)}
	encoded := t.String()
	return encoded[2:len(encoded)]
}

// getAPIResponse is a helper used to call a JSON endpoint and return the resoponse as a string
func getAPIResponse(tgtURL string, retry bool) (string, error) {
	log.Printf("Get response for: %s", tgtURL)
	apiReq, _ := http.NewRequest("GET", tgtURL, nil)
	for _, cookie := range loginCookies {
		apiReq.AddCookie(cookie)
	}
	timeout := time.Duration(10 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Do(apiReq)
	if err != nil {
		log.Printf("Unable to GET %s: %s", tgtURL, err.Error())
		return "", err
	}
	defer resp.Body.Close()

	// Forbidden... maybe cookie expired. RE-auth and try again
	if resp.StatusCode == 403 {
		lerr := jstorLogin()
		if lerr != nil {
			log.Printf("Unable to GET %s: %s", tgtURL, lerr.Error())
			return "", lerr
		}
		return getAPIResponse(tgtURL, false)
	}

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	respString := string(bodyBytes)
	if resp.StatusCode != 200 {
		return "", errors.New(respString)
	}
	return respString, nil
}

func jstorLogin() error {
	log.Printf("Logging into JSTOR...")
	// add a cookie jar to the login POST to retrieve login cookies
	cookieJar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	timeout := time.Duration(10 * time.Second)
	client := http.Client{
		Timeout: timeout,
		Jar:     cookieJar,
	}
	values := url.Values{}
	values.Set("email", jstorEmail)
	values.Add("password", jstorPass)
	reqURL := fmt.Sprintf("%s/account", jstorURL)
	loginResp, err := client.PostForm(reqURL, values)
	if err != nil {
		return err
	}

	// copy all of the cookies in the jar for future use
	loginCookies = loginResp.Cookies()

	log.Printf("Login successful")
	return nil
}

/**
 * MAIN
 */
func main() {
	log.Printf("===> Aries JSTOR service staring up <===")

	// Get config params
	log.Printf("Read configuration...")
	var port int
	flag.IntVar(&port, "port", 8080, "Aries JSTOR port (default 8080)")
	flag.StringVar(&jstorURL, "url", "https://forum.jstor.org'", "JSTOR base URL")
	flag.StringVar(&jstorProject, "project", "", "Target JSTOR project")
	flag.StringVar(&jstorEmail, "email", "", "JSTOR authorized user email")
	flag.StringVar(&jstorPass, "pass", "", "JSTOR authorized user passsword")
	flag.Parse()

	// use info above to establish a jstor login session
	logErr := jstorLogin()
	if logErr != nil {
		log.Fatalf("Unable to login to jstor: %s", logErr.Error())
		return
	}

	log.Printf("Setup routes...")
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()
	router := gin.Default()
	router.GET("/favicon.ico", favHandler)
	router.GET("/version", versionHandler)
	router.GET("/healthcheck", healthCheckHandler)
	api := router.Group("/api")
	{
		api.GET("/aries", ariesPing)
		api.GET("/aries/:id", ariesLookup)
	}

	portStr := fmt.Sprintf(":%d", port)
	log.Printf("Start Aries JSTOR v%s on port %s", version, portStr)
	log.Fatal(router.Run(portStr))
}
