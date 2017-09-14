package main

//Import Block: imports necessary libraries
import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

//Type Defenition Block: defines necessary data structures

//CTSURN is used in splitCTS, isCTSURN and functions in the Endpoint Handling Block to store CTSURN information.
type CTSURN struct {
	Stem      string
	Reference string
}

//Node is used as array(slice) in NodeResponse to store node information.
type Node struct {
	URN      []string `json:"urn"`
	Text     []string `json:"text,omitempty"`
	Previous []string `json:"previous"`
	Next     []string `json:"next"`
	Index    int      `json:"sequence"`
}

//Versions is used in ReturnCiteVersion to store version information which are added to CITEResponse for further processing.
type Versions struct {
	Texts          string `json:"texts"`
	Textcatalog    string `json:"textatalog,omitempty"`
	Citedata       string `json:"citedata,omitempty"`
	Citecatalog    string `json:"citecatalog,omitempty"`
	Citerelations  string `json:"citerelations,omitempty"`
	Citeextensions string `json:"citeextensions,omitempty"`
	DSE            string `json:"dse,omitempty"`
	ORCA           string `json:"orca,omitempty"`
}

//CITEResponse is used in ReturnCiteVersion to store cite version information and a Versions variable which are parsed to JSON format and displayed.
type CITEResponse struct {
	Status   string   `json:"status"`
	Service  string   `json:"service"`
	Versions Versions `json:"versions"`
}

//VersionResponse is used in ReturnTextsVersion to store text version information which are parsed to JSON format and displayed.
type VersionResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

//NodeResponse stores node response results from the server which are later parsed to JSON format and displayed.
type NodeResponse struct {
	RequestUrn []string `json:"requestUrn"`
	Status     string   `json:"status"`
	Service    string   `json:"service"`
	Message    string   `json:"message,omitempty"`
	URN        []string `json:"urns,omitempty"`
	Nodes      []Node   `json:""`
}

//URNResponse is used in ParseURNS to store the response data and passed to ReturnWorkURNS where it is further processed, parsed to JSON format and displayed.
type URNResponse struct {
	RequestUrn []string `json:"requestUrn"`
	Status     string   `json:"status"`
	Service    string   `json:"service"`
	Message    string   `json:"message,omitempty"`
	URN        []string `json:"urns"`
}

type CatalogResponse struct {
	RequestUrn []string       `json:"requestUrn"`
	Status     string         `json:"status"`
	Service    string         `json:"service"`
	Message    string         `json:"message,omitempty"`
	Entries    []CatalogEntry `json:`
	//Catalog		 Catalog	`json:"entries,omitempty"`
}

//Work is used in ParseWork and the Endpoint Handling Block to store information of a work and pass them to other functions.
type Work struct {
	WorkURN string
	URN     []string
	Text    []string
	Index   []int
}

//Collection is a slice of the type work. This type is not in use yet.
type Collection struct {
	Works []Work
}

type CatalogEntry struct {
	URN            string
	CitationScheme string
	GroupName      string
	WorkTitle      string
	VersionLabel   string
	ExemplarLabel  string
	Online         string
}

type Catalog struct {
	CatalogEntries []CatalogEntry
}

//CTSParams is used by parsing functions and Enpoind Handling Block functions to store the sourcetext.
type CTSParams struct {
	Sourcetext string
}

//ServerConfig is used to store the configuration that is read from config.json
type ServerConfig struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	Source     string `json:"cex_source"`
	TestSource string `json:"test_cex_source"`
}

//Helpfunction Block: These functions perform tasks that are necessary in multiple functions in the Endpoint Handling Block

//splitCTS splits the given CTS string into its Stem and Reference. Returns CTSURN.
func splitCTS(s string) CTSURN {
	var result CTSURN                                                                                         //initialize result as CTSURN
	result = CTSURN{Stem: strings.Join(strings.Split(s, ":")[0:4], ":"), Reference: strings.Split(s, ":")[4]} //first four parts go into the stem, last (fourth) part goes into reference
	return result                                                                                             //returns as CTSURN
}

//LoadConfiguration loads a JSON file defined by given string and parses it. Returns ServerConfig.
func LoadConfiguration(file string) ServerConfig {
	var config ServerConfig          //initialize config as ServerConfig
	configFile, err := os.Open(file) //attempt to open file
	defer configFile.Close()         //push closing on call list
	if err != nil {                  //error handling
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile) //initialize jsonParser with configFile
	jsonParser.Decode(&config)                //parse configFile to config
	return config                             //return ServerConfig config
}

//contains tests a string array for the occurence of a given string. Returns result as bool.
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

//isRange is used in ReturnReff and ReturnPassage to test if given URN is a range of URNs. Returns result as bool.
func isRange(s string) bool {
	switch {
	case len(strings.Split(s, ":")) < 5: //URN has to have reference to be a range
		return false
	case strings.Contains(strings.Split(s, ":")[4], "-"): //reference must contain a "-" indicating a range of URNs
		return true
	default:
		return false
	}
}

//isCTSURN is used in Endpoint Handling Block to test if given URN is valid
//The test values are length and the first two fields, which must be urn and cts. Returns result as bool.
func isCTSURN(s string) bool {
	test := strings.Split(s, ":") //initializes string array by splitting string.
	switch {
	case len(test) < 4: //URN has to have at least 4 parts
		return false
	case len(test) > 5: //URN may not have more thatn 5 parts.
		return false
	case test[0] != "urn": //First field of URN must be "urn"
		return false
	case test[1] != "cts": //Second field of URN must be "cts"
		return false
	default:
		return true
	}
}

//boolcontains tests if given bool value is contained in bool array. Reurns result as bool.
func boolcontains(s []bool, e bool) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

//level1contains tests if given string is contained in given string slice on level 1 of the URN.
//Creates array slice of bool variables that is checked by boolcontains. Returns result as bool.
func level1contains(s []string, e string) bool {
	var match []bool   //initialize match variable as bool array
	for i := range s { //go through string array. if regex matches given string plus one level
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//level1contains tests if given string is contained in given string slice on level 1 of the URN.
//Creates array slice of bool variables that is checked by boolcontains. Returns result as bool.
func level2contains(s []string, e string) bool {
	var match []bool
	for i := range s {
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//level1contains tests if given string is contained in given string slice on level 1 of the URN.
//Creates array slice of bool variables that is checked by boolcontains. Returns result as bool.
func level3contains(s []string, e string) bool {
	var match []bool
	for i := range s {
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//level1contains tests if given string is contained in given string slice on level 1 of the URN.
//Creates array slice of bool variables that is checked by boolcontains. Returns result as bool.
func level4contains(s []string, e string) bool {
	var match []bool
	for i := range s {
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//removeDuplicatesUnordered removes dublicate URNs. Returns a slice of all unique elements.
func removeDuplicatesUnordered(elements []string) []string {
	encountered := map[string]bool{}

	// Create a map of all unique elements.
	for v := range elements {
		encountered[elements[v]] = true
	}

	// Place all keys from the map into a slice.
	result := []string{}
	for key, _ := range encountered {
		result = append(result, key)
	}
	return result
}

//main function initializes the mux server. Loads the configuration from the config file, sets the serverIP.
//Maps the endpoints to the respective funtions. Initialises the headers.
func main() {
	confvar := LoadConfiguration("./config.json")
	serverIP := confvar.Port
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/cite", ReturnCiteVersion)
	router.HandleFunc("/texts", ReturnWorkURNS)
	router.HandleFunc("/texts/version", ReturnTextsVersion)
	router.HandleFunc("/catalog", ReturnCatalog)
	router.HandleFunc("/texts/first/{URN}", ReturnFirst)
	router.HandleFunc("/texts/last/{URN}", ReturnLast)
	router.HandleFunc("/texts/previous/{URN}", ReturnPrev)
	router.HandleFunc("/texts/next/{URN}", ReturnNext)
	router.HandleFunc("/texts/urns/{URN}", ReturnReff)
	router.HandleFunc("/catalog/{URN}", ReturnCatalog)
	router.HandleFunc("/texts/{URN}", ReturnPassage)
	router.HandleFunc("/{CEX}/texts/", ReturnWorkURNS)
	router.HandleFunc("/{CEX}/catalog/", ReturnCatalog)
	router.HandleFunc("/{CEX}/texts/first/{URN}", ReturnFirst)
	router.HandleFunc("/{CEX}/texts/last/{URN}", ReturnLast)
	router.HandleFunc("/{CEX}/texts/previous/{URN}", ReturnPrev)
	router.HandleFunc("/{CEX}/texts/next/{URN}", ReturnNext)
	router.HandleFunc("/{CEX}/texts/urns/{URN}", ReturnReff)
	router.HandleFunc("/{CEX}/catalog/{URN}", ReturnCatalog)
	router.HandleFunc("/{CEX}/texts/{URN}", ReturnPassage)
	router.HandleFunc("/", ReturnCiteVersion)
	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type"})
	originsOk := handlers.AllowedOrigins([]string{os.Getenv("ORIGIN_ALLOWED")})
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS"})

	log.Println("Listening at" + serverIP + "...")
	log.Fatal(http.ListenAndServe(serverIP, handlers.CORS(originsOk, headersOk, methodsOk)(router)))
}

//getContent gets the data from the given URL. Returns data as byte slice and nil if successfull, returns nil and error message in case of failure.
func getContent(url string) ([]byte, error) {
	//get response from server
	resp, err := http.Get(url)
	//return in case of GET error
	if err != nil {
		return nil, fmt.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()
	//return in case of http status error
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status error: %v", resp.StatusCode)
	}
	//reas response into byte slice
	data, err := ioutil.ReadAll(resp.Body)
	//return in case of read error
	if err != nil {
		return nil, fmt.Errorf("Read body: %v", err)
	}
	//return body data if success
	return data, nil
}

//ReturnWorkURNS prints
func ReturnWorkURNS(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
	default:
		sourcetext = confvar.TestSource
	}
	result := ParseURNS(CTSParams{Sourcetext: sourcetext})
	for i := range result.URN {
		result.URN[i] = strings.Join(strings.Split(result.URN[i], ":")[0:4], ":")
		result.URN[i] = result.URN[i] + ":"
	}
	result.URN = removeDuplicatesUnordered(result.URN)
	result.Service = "/texts"
	result.RequestUrn = []string{}
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ParseURNS(p CTSParams) URNResponse {
	input_file := p.Sourcetext
	data, err := getContent(input_file)
	if err != nil {
		return URNResponse{Status: "Exception", Message: "Couldn't open connection."}
	}

	str := string(data)
	// Remove comments
	str = strings.Split(str, "#!ctsdata")[1]
	str = strings.Split(str, "#!")[0]
	re := regexp.MustCompile("(?m)[\r\n]*^//.*$")
	str = re.ReplaceAllString(str, "")

	reader := csv.NewReader(strings.NewReader(str))
	reader.Comma = '#'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = 2

	var response URNResponse

	for {
		line, error := reader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			log.Fatal(error)
		}
		response.URN = append(response.URN, line[0])
	}
	response.Status = "Success"
	return response
}

//ParseWork extracts the relevant data out of the Sourcetext.
func ParseWork(p CTSParams) Work {
	input_file := p.Sourcetext          //get information out of Sourcetext  (string?)
	data, err := getContent(input_file) //get data out of input_file
	if err != nil {
		return Work{} //return empty work if saving in data failed
	}

	str := string(data)                           //save data in str
	str = strings.Split(str, "#!ctsdata")[1]      //split data at #!ctsdata and take the second part
	str = strings.Split(str, "#!")[0]             // split at #! and take the first part in case there is any other funtional part after #!ctsdata
	re := regexp.MustCompile("(?m)[\r\n]*^//.*$") //initialize regex to remove all newlines and carriage returns
	str = re.ReplaceAllString(str, "")            //remove unnecessary characters

	reader := csv.NewReader(strings.NewReader(str)) //initialize csv reader with str
	reader.Comma = '#'                              //set # as seperator; sits between URN and respective text
	reader.LazyQuotes = true
	reader.FieldsPerRecord = 2 //specifies that each read line will have two fields

	var response Work //initialize return value (Work)

	for {
		line, error := reader.Read() //read every line with prepared reader ([]string)
		if error == io.EOF {         //leave for loop it EOF is reached
			break
		} else if error != nil {
			log.Fatal(error) //log error
		}
		response.URN = append(response.URN, line[0])   //add first field of []line to URNs
		response.Text = append(response.Text, line[1]) //add seconf field of []line to Texts
	}
	return response
}

func ParseCatalog(p CTSParams) Catalog {
	input_file := p.Sourcetext          //get information out of Sourcetext  (string?)
	data, err := getContent(input_file) //get data out of input_file
	if err != nil {
		return Catalog{} //return empty Catalog if saving in data failed
	}

	str := string(data)                           //save data in str
	str = strings.Split(str, "#!ctscatalog")[1]   //split data at #!ctscatalog and take the second part
	str = strings.Split(str, "#!")[0]             // split at #! and take the first part in case there is any other funtional part
	re := regexp.MustCompile("(?m)[\r\n]*^//.*$") //initialize regex to remove all newlines and carriage returns
	str = re.ReplaceAllString(str, "")            //remove unnecessary characters

	reader := csv.NewReader(strings.NewReader(str)) //initialize csv reader with str
	reader.Comma = '#'                              //set # as seperator; sits between URN and respective text
	reader.LazyQuotes = true                        //check that
	reader.FieldsPerRecord = 7                      //specifies that each read line will have seven fields

	var response Catalog //initialize return value (Catalog)
	for {
		line, error := reader.Read() //read every line with prepared reader ([]string)
		if error == io.EOF {         //leave for loop it EOF is reached
			break
		} else if error != nil {
			log.Fatal(error) //log error
		}
		var entry CatalogEntry //initialize entry variable to add to Catalog
		entry.URN = line[0]    //add fields of []line to respective fields of entry
		entry.CitationScheme = line[1]
		entry.GroupName = line[2]
		entry.WorkTitle = line[3]
		entry.VersionLabel = line[4]
		entry.ExemplarLabel = line[5]
		entry.Online = line[6]
		response.CatalogEntries = append(response.CatalogEntries, entry)
	}
	return response
}

//Endpoint Handling Block: contains the handle functions that are executed according to the request.

func ReturnCiteVersion(w http.ResponseWriter, r *http.Request) {
	var result CITEResponse
	result = CITEResponse{Status: "Success",
		Service:  "/cite",
		Versions: Versions{Texts: "1.1.0", Textcatalog: ""}}
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnTextsVersion(w http.ResponseWriter, r *http.Request) {
	var result VersionResponse
	result = VersionResponse{
		Status:  "Success",
		Service: "/texts/version",
		Version: "1.1.0"}
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnFirst(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
	default:
		sourcetext = confvar.TestSource
	}
	requestUrn := vars["URN"]
	if isCTSURN(requestUrn) != true {
		message := requestUrn + " is not valid CTS."
		result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		result.Service = "/texts/first"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext})
	works := append([]string(nil), workResult.URN...)
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":")
	}
	works = removeDuplicatesUnordered(works)
	workindex := 0
	for i := range works {
		if strings.Contains(requestUrn, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestUrn == works[i]:
				workindex = i + 1
			case strings.Contains(requestUrn, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestUrn
		result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
	default:
		var RequestedWork Work
		RequestedWork.WorkURN = works[workindex-1]
		runindex := 0
		for i := range workResult.URN {
			if strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") == RequestedWork.WorkURN {
				RequestedWork.URN = append(RequestedWork.URN, workResult.URN[i])
				RequestedWork.Text = append(RequestedWork.Text, workResult.Text[i])
				runindex++
				RequestedWork.Index = append(RequestedWork.Index, runindex)
			}
		}
		result = NodeResponse{RequestUrn: []string{requestUrn},
			Status: "Success",
			Nodes: []Node{Node{URN: []string{RequestedWork.URN[0]},
				Text:  []string{RequestedWork.Text[0]},
				Next:  []string{RequestedWork.URN[1]},
				Index: RequestedWork.Index[0]}}}
	}
	result.Service = "/texts/first"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnLast(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
	default:
		sourcetext = confvar.TestSource
	}
	requestUrn := vars["URN"]
	if isCTSURN(requestUrn) != true {
		message := requestUrn + " is not valid CTS."
		result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		result.Service = "/texts/last"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext})
	works := append([]string(nil), workResult.URN...)
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":")
	}
	works = removeDuplicatesUnordered(works)
	workindex := 0
	for i := range works {
		if strings.Contains(requestUrn, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestUrn == works[i]:
				workindex = i + 1
			case strings.Contains(requestUrn, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestUrn
		result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
	default:
		var RequestedWork Work
		RequestedWork.WorkURN = works[workindex-1]
		runindex := 0
		for i := range workResult.URN {
			if strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") == RequestedWork.WorkURN {
				RequestedWork.URN = append(RequestedWork.URN, workResult.URN[i])
				RequestedWork.Text = append(RequestedWork.Text, workResult.Text[i])
				runindex++
				RequestedWork.Index = append(RequestedWork.Index, runindex)
			}
		}
		result = NodeResponse{RequestUrn: []string{requestUrn},
			Status: "Success",
			Nodes: []Node{Node{URN: []string{RequestedWork.URN[len(RequestedWork.URN)-1]},
				Text:     []string{RequestedWork.Text[len(RequestedWork.URN)-1]},
				Previous: []string{RequestedWork.URN[len(RequestedWork.URN)-2]},
				Index:    RequestedWork.Index[len(RequestedWork.URN)-1]}}}
	}
	result.Service = "/texts/last"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnPrev(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
	default:
		sourcetext = confvar.TestSource
	}
	requestUrn := vars["URN"]
	if isCTSURN(requestUrn) != true {
		message := requestUrn + " is not valid CTS."
		result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		result.Service = "/texts/previous"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext})
	works := append([]string(nil), workResult.URN...)
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":")
	}
	works = removeDuplicatesUnordered(works)
	workindex := 0
	for i := range works {
		if strings.Contains(requestUrn, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestUrn == works[i]:
				workindex = i + 1
			case strings.Contains(requestUrn, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestUrn
		result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
	default:
		var RequestedWork Work
		RequestedWork.WorkURN = works[workindex-1]
		runindex := 0
		for i := range workResult.URN {
			if strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") == RequestedWork.WorkURN {
				RequestedWork.URN = append(RequestedWork.URN, workResult.URN[i])
				RequestedWork.Text = append(RequestedWork.Text, workResult.Text[i])
				runindex++
				RequestedWork.Index = append(RequestedWork.Index, runindex)
			}
		}
		var requestedIndex int
		for i := range RequestedWork.URN {
			if RequestedWork.URN[i] == requestUrn {
				requestedIndex = i
			}
		}
		switch {
		case contains(RequestedWork.URN, requestUrn):
			switch {
			case requestedIndex == 0:
				result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: []Node{}}
			case requestedIndex-1 == 0:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex-1]},
						Text:  []string{RequestedWork.Text[requestedIndex-1]},
						Next:  []string{RequestedWork.URN[requestedIndex]},
						Index: RequestedWork.Index[requestedIndex-1]}}}
			default:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex-1]},
						Text:     []string{RequestedWork.Text[requestedIndex-1]},
						Next:     []string{RequestedWork.URN[requestedIndex]},
						Previous: []string{RequestedWork.URN[requestedIndex-2]},
						Index:    RequestedWork.Index[requestedIndex-1]}}}
			}
		default:
			message := "Could not find node to " + requestUrn + " in source."
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		}
	}
	result.Service = "/texts/previous"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnNext(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
	default:
		sourcetext = confvar.TestSource
	}
	requestUrn := vars["URN"]
	if isCTSURN(requestUrn) != true {
		message := requestUrn + " is not valid CTS."
		result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		result.Service = "/texts/next"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext})
	works := append([]string(nil), workResult.URN...)
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":")
	}
	works = removeDuplicatesUnordered(works)
	workindex := 0
	for i := range works {
		if strings.Contains(requestUrn, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestUrn == works[i]:
				workindex = i + 1
			case strings.Contains(requestUrn, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestUrn
		result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
	default:
		var RequestedWork Work
		RequestedWork.WorkURN = works[workindex-1]
		runindex := 0
		for i := range workResult.URN {
			if strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") == RequestedWork.WorkURN {
				RequestedWork.URN = append(RequestedWork.URN, workResult.URN[i])
				RequestedWork.Text = append(RequestedWork.Text, workResult.Text[i])
				runindex++
				RequestedWork.Index = append(RequestedWork.Index, runindex)
			}
		}
		var requestedIndex int
		for i := range RequestedWork.URN {
			if RequestedWork.URN[i] == requestUrn {
				requestedIndex = i
			}
		}
		switch {
		case contains(RequestedWork.URN, requestUrn):
			switch {
			case requestedIndex == len(RequestedWork.URN)-1:
				result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: []Node{}}
			case requestedIndex+1 == len(RequestedWork.URN)-1:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex+1]},
						Text:     []string{RequestedWork.Text[requestedIndex+1]},
						Previous: []string{RequestedWork.URN[requestedIndex]},
						Index:    RequestedWork.Index[requestedIndex+1]}}}
			default:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex+1]},
						Text:     []string{RequestedWork.Text[requestedIndex+1]},
						Next:     []string{RequestedWork.URN[requestedIndex+2]},
						Previous: []string{RequestedWork.URN[requestedIndex]},
						Index:    RequestedWork.Index[requestedIndex+1]}}}
			}
		default:
			message := "Could not find node to " + requestUrn + " in source."
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		}
	}
	result.Service = "/texts/next"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnReff(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json") //load configuration from json file (ServerConfig)
	vars := mux.Vars(r)                         //load vars from mux config to get CEX and URN information )[]string ?)
	requestCEX := ""                            //initalize CEX variable (string)
	requestCEX = vars["CEX"]                    //save CEX name in CEX variable
	var sourcetext string                       //initialize sourcetext variable; will hold CEX data
	switch {                                    //switch to determine wether a CEX file was specified
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex" //build URL to CEX file if CEX file was specified
	default:
		sourcetext = confvar.TestSource //use TestSource as URL to CEX-file as found in config.json
	}
	requestUrn := vars["URN"]         //safe requested URN
	if isCTSURN(requestUrn) != true { //test if given URN is valid (bool)
		message := requestUrn + " is not valid CTS."                                                    //build message part of NodeResponse
		result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message} //building result (NodeResponse)
		result.Service = "/texts/urns"                                                                  // adding Service part to result (NodeResponse)
		resultJSON, _ := json.Marshal(result)                                                           //parsing result to JSON format (_ would contain err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")                               //set output format
		fmt.Fprintln(w, string(resultJSON))                                                             //output
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext}) //parse the work
	works := append([]string(nil), workResult.URN...)          // append URNs from workResult to works
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") //crop URNs in []work to first four parts of URN (why?)
	}
	works = removeDuplicatesUnordered(works) //remove dublicate URNS
	workindex := 0                           //initialize variable to save index of works to work with
	for i := range works {
		if strings.Contains(requestUrn, works[i]) {
			teststring := works[i] + ":" //add colon which was lost during joins
			switch {
			case requestUrn == works[i]:
				workindex = i + 1
			case strings.Contains(requestUrn, teststring): //should have matched before already, shouldn't it?
				workindex = i + 1
			}
		}
	}
	var result URNResponse //initialize result (URNResponse)
	switch {
	case workindex == 0: //if requested URN is not among URNs in works prepare and display message accordingly
		message := "No results for " + requestUrn
		result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		result.Service = "/texts/urns"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
	default: // if requested URN is among URNs in work
		var RequestedWork Work
		RequestedWork.WorkURN = works[workindex-1]
		runindex := 0
		for i := range workResult.URN {
			if strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") == RequestedWork.WorkURN {
				RequestedWork.URN = append(RequestedWork.URN, workResult.URN[i])
				RequestedWork.Text = append(RequestedWork.Text, workResult.Text[i])
				runindex++
				RequestedWork.Index = append(RequestedWork.Index, runindex)
			}
		}
		switch {
		case isRange(requestUrn): //if range is requested,
			ctsurn := splitCTS(requestUrn)                   //split URN into its stem and reference
			ctsrange := strings.Split(ctsurn.Reference, "-") //split reference along the hyphen
			startURN := ctsurn.Stem + ":" + ctsrange[0]      //define startURN as the first field
			endURN := ctsurn.Stem + ":" + ctsrange[1]        //definde endURN as the second field
			var startindex, endindex int
			switch { //find startindex in RequestedWork.URN
			case contains(RequestedWork.URN, startURN): //if the startURN is in the URNs of RequestedWork use its index as startindex
				for i := range RequestedWork.URN {
					if RequestedWork.URN[i] == startURN {
						startindex = i
					}
				}
			case level1contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			case level2contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			case level3contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			case level4contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			default:
				startindex = 0
			}
			switch { //find endindex in RequestedWork.URN
			case contains(RequestedWork.URN, endURN):
				for i := range RequestedWork.URN {
					if RequestedWork.URN[i] == endURN {
						endindex = i
					}
				}
			case level1contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			case level2contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			case level3contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			case level4contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			default:
				endindex = len(RequestedWork.URN) - 1
			}
			range_urn := RequestedWork.URN[startindex : endindex+1]                                   //safe requested URNS in variable range_urn
			result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: range_urn} //assemble result
			result.Service = "/texts/urns"
			resultJSON, _ := json.Marshal(result)                             //parse result to json format
			w.Header().Set("Content-Type", "application/json; charset=utf-8") //set output format
			fmt.Fprintln(w, string(resultJSON))                               //output
		default:
			switch {
			case contains(RequestedWork.URN, requestUrn):
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: []string{requestUrn}}
			case level1contains(RequestedWork.URN, requestUrn):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: matchingURNs}
			case level2contains(RequestedWork.URN, requestUrn):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: matchingURNs}
			case level3contains(RequestedWork.URN, requestUrn):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: matchingURNs}
			case level4contains(RequestedWork.URN, requestUrn):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: matchingURNs}
			default:
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: "Couldn't find URN."}
			}
			result.Service = "/texts/urns"
			resultJSON, _ := json.Marshal(result)                             //parse result to json format
			w.Header().Set("Content-Type", "application/json; charset=utf-8") //set output format
			fmt.Fprintln(w, string(resultJSON))                               //output
		}
	}
}

func ReturnPassage(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
	default:
		sourcetext = confvar.TestSource
	}
	requestUrn := vars["URN"]
	if isCTSURN(requestUrn) != true {
		message := requestUrn + " is not valid CTS."
		result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		result.Service = "/texts"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext})
	works := append([]string(nil), workResult.URN...)
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":")
	}
	works = removeDuplicatesUnordered(works)
	workindex := 0
	for i := range works {
		if strings.Contains(requestUrn, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestUrn == works[i]:
				workindex = i + 1
			case strings.Contains(requestUrn, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestUrn
		result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
	default:
		var RequestedWork Work
		RequestedWork.WorkURN = works[workindex-1]
		runindex := 0
		for i := range workResult.URN {
			if strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") == RequestedWork.WorkURN {
				RequestedWork.URN = append(RequestedWork.URN, workResult.URN[i])
				RequestedWork.Text = append(RequestedWork.Text, workResult.Text[i])
				runindex++
				RequestedWork.Index = append(RequestedWork.Index, runindex)
			}
		}
		var requestedIndex int
		for i := range RequestedWork.URN {
			if RequestedWork.URN[i] == requestUrn {
				requestedIndex = i
			}
		}
		switch {
		case contains(RequestedWork.URN, requestUrn):
			switch {
			case requestedIndex == 0:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex]},
						Text:  []string{RequestedWork.Text[requestedIndex]},
						Next:  []string{RequestedWork.URN[requestedIndex+1]},
						Index: RequestedWork.Index[requestedIndex]}}}
			case requestedIndex == len(RequestedWork.URN)-1:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex]},
						Text:     []string{RequestedWork.Text[requestedIndex]},
						Previous: []string{RequestedWork.URN[requestedIndex-1]},
						Index:    RequestedWork.Index[requestedIndex]}}}
			default:
				result = NodeResponse{RequestUrn: []string{requestUrn},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex]},
						Text:     []string{RequestedWork.Text[requestedIndex]},
						Next:     []string{RequestedWork.URN[requestedIndex+1]},
						Previous: []string{RequestedWork.URN[requestedIndex-1]},
						Index:    RequestedWork.Index[requestedIndex]}}}
			}
		case level1contains(RequestedWork.URN, requestUrn):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
				match = append(match, match2)
			}
			for i := range match {
				if match[i] == true {
					previousnode := ""
					nextnode := ""
					if RequestedWork.Index[i] > 1 {
						previousnode = RequestedWork.URN[RequestedWork.Index[i]-2]
					}
					if RequestedWork.Index[i] < len(RequestedWork.URN) {
						nextnode = RequestedWork.URN[RequestedWork.Index[i]]
					}
					matchingNodes = append(matchingNodes, Node{URN: []string{RequestedWork.URN[i]}, Text: []string{RequestedWork.Text[i]}, Previous: []string{previousnode}, Next: []string{nextnode}, Index: RequestedWork.Index[i]})
				}
			}
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: matchingNodes}
		case level2contains(RequestedWork.URN, requestUrn):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
				match = append(match, match2)
			}
			for i := range match {
				if match[i] == true {
					previousnode := ""
					nextnode := ""
					if RequestedWork.Index[i] > 1 {
						previousnode = RequestedWork.URN[RequestedWork.Index[i]-2]
					}
					if RequestedWork.Index[i] < len(RequestedWork.URN) {
						nextnode = RequestedWork.URN[RequestedWork.Index[i]]
					}
					matchingNodes = append(matchingNodes, Node{URN: []string{RequestedWork.URN[i]}, Text: []string{RequestedWork.Text[i]}, Previous: []string{previousnode}, Next: []string{nextnode}, Index: RequestedWork.Index[i]})
				}
			}
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: matchingNodes}
		case level3contains(RequestedWork.URN, requestUrn):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
				match = append(match, match2)
			}
			for i := range match {
				if match[i] == true {
					previousnode := ""
					nextnode := ""
					if RequestedWork.Index[i] > 1 {
						previousnode = RequestedWork.URN[RequestedWork.Index[i]-2]
					}
					if RequestedWork.Index[i] < len(RequestedWork.URN) {
						nextnode = RequestedWork.URN[RequestedWork.Index[i]]
					}
					matchingNodes = append(matchingNodes, Node{URN: []string{RequestedWork.URN[i]}, Text: []string{RequestedWork.Text[i]}, Previous: []string{previousnode}, Next: []string{nextnode}, Index: RequestedWork.Index[i]})
				}
			}
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: matchingNodes}
		case level4contains(RequestedWork.URN, requestUrn):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestUrn + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
				match = append(match, match2)
			}
			for i := range match {
				if match[i] == true {
					previousnode := ""
					nextnode := ""
					if RequestedWork.Index[i] > 1 {
						previousnode = RequestedWork.URN[RequestedWork.Index[i]-2]
					}
					if RequestedWork.Index[i] < len(RequestedWork.URN) {
						nextnode = RequestedWork.URN[RequestedWork.Index[i]]
					}
					matchingNodes = append(matchingNodes, Node{URN: []string{RequestedWork.URN[i]}, Text: []string{RequestedWork.Text[i]}, Previous: []string{previousnode}, Next: []string{nextnode}, Index: RequestedWork.Index[i]})
				}
			}
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: matchingNodes}
		case isRange(requestUrn):
			var rangeNodes []Node
			ctsurn := splitCTS(requestUrn)
			ctsrange := strings.Split(ctsurn.Reference, "-")
			startURN := ctsurn.Stem + ":" + ctsrange[0]
			endURN := ctsurn.Stem + ":" + ctsrange[1]
			var startindex, endindex int
			switch {
			case contains(RequestedWork.URN, startURN):
				for i := range RequestedWork.URN {
					if RequestedWork.URN[i] == startURN {
						startindex = i
					}
				}
			case level1contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			case level2contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			case level3contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			case level4contains(RequestedWork.URN, startURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((startURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						startindex = i
						break
					}
				}
			default:
				startindex = 0
			}
			switch {
			case contains(RequestedWork.URN, endURN):
				for i := range RequestedWork.URN {
					if RequestedWork.URN[i] == endURN {
						endindex = i
					}
				}
			case level1contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			case level2contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			case level3contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			case level4contains(RequestedWork.URN, endURN):
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((endURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := len(match) - 1; i >= 0; i-- {
					if match[i] == true {
						endindex = i
						break
					}
				}
			default:
				endindex = len(RequestedWork.URN) - 1
			}
			range_urn := RequestedWork.URN[startindex : endindex+1]
			range_text := RequestedWork.Text[startindex : endindex+1]
			range_index := RequestedWork.Index[startindex : endindex+1]
			for i := range range_urn {
				previousnode := ""
				nextnode := ""
				if range_index[i] > 1 {
					previousnode = RequestedWork.URN[range_index[i]-2]
				}
				if range_index[i] < len(RequestedWork.URN) {
					nextnode = RequestedWork.URN[range_index[i]]
				}
				rangeNodes = append(rangeNodes, Node{URN: []string{range_urn[i]}, Text: []string{range_text[i]}, Previous: []string{previousnode}, Next: []string{nextnode}, Index: range_index[i]})
			}
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Success", Nodes: rangeNodes}
		default:
			message := "Could not find node to " + requestUrn + " in source."
			result = NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
		}
	}
	result.Service = "/texts"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnCatalog(w http.ResponseWriter, r *http.Request) {
	confvar := LoadConfiguration("config.json") //load configuration from json file (ServerConfig)
	vars := mux.Vars(r)                         //load vars from mux config to get CEX and URN information )[]string ?)
	requestCEX := ""                            //initalize CEX variable (string)
	requestCEX = vars["CEX"]                    //save CEX name in CEX variable
	var sourcetext string                       //initialize sourcetext variable; will hold CEX data
	switch {                                    //switch to determine wether a CEX file was specified
	case requestCEX != "": //either {CEX}/catalog/ or /{CEX}/catalog/{URN}
		sourcetext = confvar.Source + requestCEX + ".cex" //build URL to CEX file if CEX file was specified
	default:
		sourcetext = confvar.TestSource //use TestSource as URL to CEX-file as found in config.json
	}
	requestUrn := ""         //initialize requestURN --> Check if this os ;eft empty!
	requestUrn = vars["URN"] //safe requested URN

	switch {
	case requestUrn != "":

		if isCTSURN(requestUrn) != true { //test if given URN is valid (bool)
			message := requestUrn + " is not valid CTS."                                                    //build message part of NodeResponse
			result := NodeResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message} //building result (NodeResponse)
			result.Service = "/texts/urns"                                                                  // adding Service part to result (NodeResponse)
			resultJSON, _ := json.Marshal(result)                                                           //parsing result to JSON format (_ would contain err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")                               //set output format
			fmt.Fprintln(w, string(resultJSON))                                                             //output
			return
		}

		catalogResult := ParseCatalog(CTSParams{Sourcetext: sourcetext}) //parse the catalog
		entries := catalogResult.CatalogEntries
		var urns []string
		for i := range entries {
			urns = append(urns, entries[i].URN) //initialize works variable
		}
		urns = removeDuplicatesUnordered(urns)

	default:

		/*
				works := append([]string(nil), catalogResult.CatalogEntries.URN...) // append URNs from catalogResult to works
				works = removeDuplicatesUnordered(works)                            //remove dublicate URNS (necessary?)

			workindex := 0 //initialize variable to save index of works to work with
			for i := range works {
				if strings.Contains(requestUrn, works[i]) {
					teststring := works[i] + ":" //why do this? add colon which was lost during joins
					switch {
					case requestUrn == works[i]:
						workindex = i + 1
					case strings.Contains(requestUrn, teststring): //should have matched before already, shouldn't it?
						workindex = i + 1
					}
				}
			}
			var result URNResponse //initialize result (URNResponse)
			switch {
			case workindex == 0: //if requested URN is not among URNs in works prepare and display message accordingly
				message := "No results for " + requestUrn
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: message}
				result.Service = "/texts/urns"
				resultJSON, _ := json.Marshal(result)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				fmt.Fprintln(w, string(resultJSON))
			default:
				var RequestedWork Work
				RequestedWork.WorkURN = works[workindex-1]
				runindex := 0
				for i := range catalogResult.URN {
					if catalogResult.URN[i] == RequestedWork.WorkURN {
						RequestedWork.URN = append(RequestedWork.URN, catalogResult.URN[i])
						runindex++
						RequestedWork.Index = append(RequestedWork.Index, runindex)
					}
				}
			}
			switch {
			case contains(RequestedWork.URN, requestUrn):
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Success", URN: []string{requestUrn}}
			default:
				result = URNResponse{RequestUrn: []string{requestUrn}, Status: "Exception", Message: "Couldn't find URN."}
			}
			result.Service = "/texts/urns"
			resultJSON, _ := json.Marshal(result)                             //parse result to json format
			w.Header().Set("Content-Type", "application/json; charset=utf-8") //set output format
			fmt.Fprintln(w, string(resultJSON))                               //output

		*/
	}
}
