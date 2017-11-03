package main

//***Import Block: imports necessary libraries***

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
	//	"bytes"
	"path/filepath"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

//var logFileName string

//***Type Defenition Block: defines necessary data structures***

//Stores CTSURN information. Used in splitCTS, isCTSURN and functions in the Endpoint Handling Block.
type CTSURN struct {
	Stem      string
	Reference string
}

//Stores Node information. Used in NodeResponse.
type Node struct {
	URN      []string `json:"urn"`
	Text     []string `json:"text,omitempty"`
	Previous []string `json:"previous"`
	Next     []string `json:"next"`
	Index    int      `json:"sequence"`
}

//Stores version information which are added to CITEResponse for further processing. Used in ReturnCiteVersion
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

//Stores cite version information and a versions variable which are parsed to JSON format and displayed. Used in ReturnCiteVersion
type CITEResponse struct {
	Status   string   `json:"status"`
	Service  string   `json:"service"`
	Versions Versions `json:"versions"`
}

//Stores text version information which are parsed to JSON format and displayed. Used in ReturnTextsVersion
type VersionResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

//Stores node response results which are later parsed to JSON format and displayed. Used throughout the Endpoint Handling Block.
type NodeResponse struct {
	requestURN []string `json:"requestURN"`
	Status     string   `json:"status"`
	Service    string   `json:"service"`
	Message    string   `json:"message,omitempty"`
	URN        []string `json:"urns,omitempty"`
	Nodes      []Node   `json:""`
}

//Stores URN response results, which are passed to ReturnWorkURNS for further processing, parsing to JSON format and displaying. Used in ParseURNSFromCTSdata.
type URNResponse struct {
	requestURN []string `json:"requestURN"`
	Status     string   `json:"status"`
	Service    string   `json:"service"`
	Message    string   `json:"message,omitempty"`
	URN        []string `json:"urns"`
}

//Stores catalog response results, which are parsed to JSON format and displayed. Used in ReturnCatalog.
type CatalogResponse struct {
	Status  string   `json:"status"`
	Service string   `json:"service"`
	Message string   `json:"message,omitempty"`
	URN     []string `json:"urns"`
}

//Stores work information for transfer to other functions. Used in ParseWork and the Endpoint Handling Block.
type Work struct {
	WorkURN string
	URN     []string
	Text    []string
	Index   []int
}

//Holds multiple Works. Not in use yet.
type Collection struct {
	Works []Work
}

//Stores CEX catalog entry information. Format of CEX catalog seems not to be fixed yet...
type CatalogEntry struct {
	URN            string
	CitationScheme string
	GroupName      string
	WorkTitle      string
	VersionLabel   string
	ExemplarLabel  string
	Online         string
	Lang           string
}

//Stores catalog entries. Used in ParseCatalog to transfer results to ReturnCatalog.
type Catalog struct {
	CatalogEntries []CatalogEntry
}

//Stores a sourcetext. Used by parsing functions and in Endpoint Handling Block.
type CTSParams struct {
	Sourcetext string
}

//Stores server configuration. Used in all functions that need access to server parameters and the source.
type ServerConfig struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	Source     string `json:"cex_source"`
	TestSource string `json:"test_cex_source"`
}

//***Helpfunction Block: These functions perform tasks that are necessary in multiple functions in the Endpoint Handling Block***

//Splits CTS string s into its Stem and Reference. Returns CTSURN.
func splitCTS(stringToBeSplit string) CTSURN {
	var result CTSURN                                                                                                                     //initialize result as CTSURN
	result = CTSURN{Stem: strings.Join(strings.Split(stringToBeSplit, ":")[0:4], ":"), Reference: strings.Split(stringToBeSplit, ":")[4]} //first four parts go into the stem, last (fourth) part goes into reference
	return result                                                                                                                         //returns as CTSURN
}

//Loads and parses JSON file defined by string s. Returns ServerConfig.
func LoadConfiguration(file string) ServerConfig {
	var config ServerConfig                    //initialize config as ServerConfig
	configFile, openFileError := os.Open(file) //attempt to open file
	defer configFile.Close()                   //push closing on call list
	if openFileError != nil {                  //error handling
		fmt.Println("Open file error: " + openFileError.Error())
	}
	jsonParser := json.NewDecoder(configFile) //initialize jsonParser with configFile
	jsonParser.Decode(&config)                //parse configFile to config
	return config                             //return ServerConfig config
}

//Returns bool for whether string slice stringSlice contains string e.
func contains(stringSlice []string, testedString string) bool {
	for _, currentString := range stringSlice {
		if currentString == testedString {
			return true
		}
	}
	return false
}

//Returns bool for whether string s resembles a URN-range. Called in ReturnReff and ReturnPassage.
func isRange(s string) bool {
	switch {
	case len(strings.Split(s, ":")) < 5: //Test whether URN has reference..
		return false //..else its not a range.
	case strings.Contains(strings.Split(s, ":")[4], "-"): //The Reference must contain a hyphen ('-') indicating a range of URNs.
		return true //Than it should be a range.
	default: //In any other case..
		return false //...its not a CTS URN range.
	}
}

//Returns bool for whether length and structure of string s indicate it is a valid CTS URN. Called in Endpoint Handling Block.
func isCTSURN(s string) bool {
	log.Println("Testing whether \"" + s + "\" is a valid CTS URN")
	test := strings.Split(s, ":") //initializes string array by splitting string into functional parts.
	switch {
	case len(test) < 4: //URN has to have at least 4 parts
		log.Println("Not a valid CTS URN: not enough fields. (Should be 4 or 5)")
		return false
	case len(test) > 5: //URN may not have more thatn 5 parts.
		log.Println("Not a valid CTS URN: too many fields. (Should be 4 or 5)")
		return false
	case test[0] != "urn": //First field of URN must be "urn"
		log.Println("Not a valid CTS URN: first field must be urn")
		return false
	case test[1] != "cts": //Second field of URN must be "cts"
		log.Println("Not a valid CTS URN: second field must be cts")
		return false
	default:
		log.Println("CTS URN is valid")
		return true
	}
}

//Returns bool for whether bool e is contained in bool slice s.
func boolcontains(s []bool, e bool) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

//Returns bool for whether string e is contained in string slice s on level 1 of the URN.
func level1contains(s []string, e string) bool {
	var match []bool   //initialize bool array match
	for i := range s { //go through string array. if regex matches string plus one level
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//Returns bool for whether string e is contained in string slice s on level 2 of the URN.
func level2contains(s []string, e string) bool {
	var match []bool
	for i := range s {
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//Returns bool for whether string e is contained in string slice s on level 3 of the URN.
func level3contains(s []string, e string) bool {
	var match []bool
	for i := range s {
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//Returns bool for whether string e is contained in string slice s on level 4 of the URN.
func level4contains(s []string, e string) bool {
	var match []bool
	for i := range s {
		match2, _ := regexp.MatchString((e + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), s[i])
		match = append(match, match2)
	}
	return boolcontains(match, true)
}

//Removes dublicate URNs from elements. Returns a slice of all unique elements.
func removeDuplicatesUnordered(elements []string) []string {
	encountered := map[string]bool{} //initalize bool map with string keys
	// Create a map of all unique elements.
	for v := range elements {
		encountered[elements[v]] = true //all elements are set to true
	}
	// Place all keys from the map into a slice.
	result := []string{}
	for key, _ := range encountered {
		result = append(result, key) //append every key to the slice
	}
	return result
}

//***Main Block***

//Initializes mux server, loads configuration from config file, sets the serverIP, maps endpoints to respective funtions. Initialises the headers.
func main() {
	log.Println("Starting up local server.")
	confvar := LoadConfiguration("./config.json")
	serverIP := confvar.Port //should be called serverPort instead
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

	log.Println("Server is running")
	log.Println("Listening at" + serverIP + "...")

	logFileName := (time.Now().Format("2006_01_02_15-04-05" + ".log"))
	logFilePath := ("." + string(filepath.Separator) + "logs" + string(filepath.Separator))

	if _, err := os.Stat(logFilePath); err != nil {
		if os.IsNotExist(err) {
			os.Mkdir(logFilePath, os.ModePerm)
		} else {
			fmt.Errorf("Error creating folder: " + logFilePath)
		}
	}

	logFile := (logFilePath + logFileName)

	openedLogFile, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Errorf("Could not open logfile %v", err)
	}
	defer openedLogFile.Close()

	log.Println("Logging to file: " + logFile)
	log.SetOutput(openedLogFile)

	log.Fatal(http.ListenAndServe(serverIP, handlers.CORS(originsOk, headersOk, methodsOk)(router)))
}

//Fetches data from the url. Returns byte slice. Error handling implemented.
func getContent(url string) ([]byte, error) {
	resp, err := http.Get(url) //get response from server
	if err != nil {
		return nil, fmt.Errorf("GET error: %v", err) //return in case of GET error
	}
	defer resp.Body.Close() //return in case of http status error
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status error: %v", resp.StatusCode)
	}
	data, err := ioutil.ReadAll(resp.Body) //read response into byte slice
	if err != nil {                        //return in case of read error
		return nil, fmt.Errorf("Read body: %v", err)
	}
	return data, nil
}

//Parsing Block: contains functions to parse the different parts of the CEX file

//Parses the text in order to extract the URNS from #!ctsdata. Returns URNResponse. Called by ReturnWorkURNS
func ParseURNSFromCTSdata(ctsParams CTSParams) URNResponse {
	log.Println("Parsing URNS from #!ctsdata")
	input_file := ctsParams.Sourcetext
	data, err := getContent(input_file)
	if err != nil {
		log.Println("Error opening connection")
		log.Println("No URNS parsed")
		return URNResponse{Status: "Exception", Message: "Couldn't open file. No internet connection?"}
	}

	str := string(data)
	str = strings.Split(str, "#!ctsdata")[1]
	str = strings.Split(str, "#!")[0]
	re := regexp.MustCompile("(?m)[\r\n]*^//.*$")
	str = re.ReplaceAllString(str, "")

	reader := csv.NewReader(strings.NewReader(str))
	reader.Comma = '#'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = 2

	var urnResponse URNResponse

	for {
		line, readError := reader.Read()
		if readError == io.EOF {
			break
		} else if readError != nil {
			panic(readError)
			//log.Fatal(error)
		}
		urnResponse.URN = append(urnResponse.URN, line[0])
	}
	urnResponse.Status = "Success"
	log.Println("URNS parsed succesfully")
	return urnResponse
}

//Extracts URN and corresponding text out of the Sourcetext. Returns Work. Called by ReturnFirst, ReturnLast, ReturnPrev, ReturnReff, ReturnPassage.
func ParseWork(ctsParams CTSParams) Work {
	log.Println("Parsing work")
	//input_file := ctsParams.Sourcetext          //get filename out of Sourcetext  (string?)
	//data, err := getContent(input_file) //get data out of input_file
	var response Work                                         //initialize return value (Work)
	data, getContentError := getContent(ctsParams.Sourcetext) // save a line of code
	if getContentError != nil {
		panic(getContentError) //panic if content could not be retrieved
		//	log.Fatal(err) //log error
		return response //return empty work if saving in data failed
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

	for {
		line, readError := reader.Read() //read every line with prepared reader ([]string)
		if readError == io.EOF {         //leave for loop it EOF is reached
			break
		} else if readError != nil {
			panic(readError) //panic if lines could not be read
			//log.Fatal(error) //log error
		}
		response.URN = append(response.URN, line[0])   //add first field of []line to URNs
		response.Text = append(response.Text, line[1]) //add second field of []line to Texts
	}
	log.Println("Work parsed succesfully")
	return response
}

func ParseCatalog(ctsParams CTSParams) Catalog {
	log.Println("Parsing catalog")
	input_file := ctsParams.Sourcetext              //get information out of Sourcetext  (string?)
	data, getContentError := getContent(input_file) //get data out of input_file
	if getContentError != nil {
		log.Println("Parsing Catalog failed.")
		panic(getContentError)
	}

	str := string(data) //save data in str
	//ToDo: add regex that tests for #!ctscatalog 	re :=regexp.MatchString("#!ctscatalog", str)
	str = strings.Split(str, "#!ctscatalog")[1]   //split data at #!ctscatalog and take the second part
	str = strings.Split(str, "#!")[0]             // split at #! and take the first part in case there is any other funtional part
	re := regexp.MustCompile("(?m)[\r\n]*^//.*$") //initialize regex to remove all newlines and carriage returns
	str = re.ReplaceAllString(str, "")            //remove unnecessary characters
	//	log.Println("String: " + str)
	reader := csv.NewReader(strings.NewReader(str)) //initialize csv reader with str
	reader.Comma = '#'                              //set # as seperator; sits between URN and respective text
	reader.LazyQuotes = true
	reader.FieldsPerRecord = 8 //specifies that each read line will have as many fields as first line
	var response Catalog       //initialize return value (Catalog)
	for {
		line, readError := reader.Read() //read every line with prepared reader ([]string)
		if readError == io.EOF {         //leave the loop if EOF is reached
			break
		} else if readError != nil {
			//log.Fatal(error) //log error
			panic(readError)
		}
		var entry CatalogEntry //initialize entry variable to add to Catalog
		entry.URN = line[0]    //add fields of []line to respective fields of entry
		entry.CitationScheme = line[1]
		entry.GroupName = line[2]
		entry.WorkTitle = line[3]
		entry.VersionLabel = line[4]
		entry.ExemplarLabel = line[5]
		entry.Online = line[6]
		entry.Lang = line[7]
		if isCTSURN(entry.URN) {
			response.CatalogEntries = append(response.CatalogEntries, entry)
		}
	}
	log.Println("Catalog parsed succesfully")
	return response
}

//Endpoint Handling Block: contains the handle functions that are executed according to the request.

func ReturnCiteVersion(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnCiteVersion")
	var result CITEResponse
	result = CITEResponse{Status: "Success",
		Service:  "/cite",
		Versions: Versions{Texts: "1.1.0", Textcatalog: ""}}
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	log.Println("ReturnCiteVersion executed succesfully")
	fmt.Fprintln(w, string(resultJSON))
}

//ReturnWorkURNS returns the URNs as found in the #!ctsdata block of the CEX file
func ReturnWorkURNS(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rwuError := recover(); rwuError != nil {
			message := ("Error encountered. Please contact development team and send in current logfile!") //build message part of NodeResponse
			result := NodeResponse{requestURN: []string{}, Status: "Exception", Message: message}          //building result (NodeResponse)
			result.Service = "/texts"                                                                      // adding Service part to result (NodeResponse)
			resultJSON, _ := json.Marshal(result)                                                          //parsing result to JSON format
			w.Header().Set("Content-Type", "application/json; charset=utf-8")                              //set output format
			fmt.Fprintln(w, string(resultJSON))
			log.Println("Error encountered: \"", rwuError, "\"")
			// return //necessary?
		}
	}()

	log.Println("Called function: ReturWorkURNS") //log what function is doing
	confvar := LoadConfiguration("config.json")   //load configuration from configfile
	vars := mux.Vars(r)                           //load configuration from mux server
	requestCEX := ""
	requestCEX = vars["CEX"] //set requestCEX variable to value from mux server
	var sourcetext string    //create sourcetext variable
	switch {                 //check where CEX file was specified
	case requestCEX != "": //if specified in URL, use that file
		sourcetext = confvar.Source + requestCEX + ".cex"
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default: //if not: use CEX file specified in configfile
		sourcetext = confvar.TestSource
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from congfig instead.")
	}
	urnResponse := ParseURNSFromCTSdata(CTSParams{Sourcetext: sourcetext})
	for i := range urnResponse.URN {
		urnResponse.URN[i] = strings.Join(strings.Split(urnResponse.URN[i], ":")[0:4], ":")
		urnResponse.URN[i] = urnResponse.URN[i] + ":"
	}
	urnResponse.URN = removeDuplicatesUnordered(urnResponse.URN)
	urnResponse.Service = "/texts"
	urnResponse.requestURN = []string{} //creates empty string?
	resultJSON, _ := json.Marshal(urnResponse)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
	if urnResponse.Status == "Success" {
		log.Println("ReturWorkURNS executed succesfully")
	} else {
		log.Println("ReturWorkURNS executed with Exception")
	}
}

func ReturnTextsVersion(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnTextsVersion")
	var result VersionResponse
	result = VersionResponse{
		Status:  "Success",
		Service: "/texts/version",
		Version: "1.1.0"}
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	log.Println("ReturnTextsVersion executed succesfully")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnFirst(w http.ResponseWriter, r *http.Request) {

	defer func() {
		if rfError := recover(); rfError != nil {
			message := ("Error encountered. Please contact development team and send in current logfile!") //build message part of NodeResponse
			result := NodeResponse{requestURN: []string{}, Status: "Exception", Message: message}          //building result (NodeResponse)
			result.Service = "/texts/first"                                                                // adding Service part to result (NodeResponse)
			resultJSON, _ := json.Marshal(result)                                                          //parsing result to JSON format
			w.Header().Set("Content-Type", "application/json; charset=utf-8")                              //set output format
			fmt.Fprintln(w, string(resultJSON))
			log.Println("Error encountered: \"", rfError, "\"")
			// return //necessary?
		}
	}()
	log.Println("Called function: ReturnFirst")
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from config instead.")
	}
	requestURN := vars["URN"]
	//log.Println("Requested URN: " + requestURN)
	if isCTSURN(requestURN) != true {
		message := requestURN + " is not valid CTS."
		result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		result.Service = "/texts/first"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		log.Println("ReturnFirst executed with exeption")
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
		if strings.Contains(requestURN, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestURN == works[i]:
				workindex = i + 1
			case strings.Contains(requestURN, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestURN
		log.Println("Requested URN not in works. Returning exception message")
		result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
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
		result = NodeResponse{requestURN: []string{requestURN},
			Status: "Success",
			Nodes: []Node{Node{URN: []string{RequestedWork.URN[0]},
				Text:  []string{RequestedWork.Text[0]},
				Next:  []string{RequestedWork.URN[1]},
				Index: RequestedWork.Index[0]}}}
	}
	result.Service = "/texts/first"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	log.Println("ReturnFirst executed succesfully")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnLast(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnLast")
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from config instead.")
	}
	requestURN := vars["URN"]
	if isCTSURN(requestURN) != true {
		message := requestURN + " is not valid CTS."
		result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		result.Service = "/texts/last"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		log.Println("ReturnLast executed succesfully")
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
		if strings.Contains(requestURN, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestURN == works[i]:
				workindex = i + 1
			case strings.Contains(requestURN, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestURN
		result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
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
		result = NodeResponse{requestURN: []string{requestURN},
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
	log.Println("Called function: ReturnPrev")
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from config instead.")
	}
	requestURN := vars["URN"]
	if isCTSURN(requestURN) != true {
		message := requestURN + " is not valid CTS."
		result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		result.Service = "/texts/previous"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		log.Println("ReturnReff executed succesfully")
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
		if strings.Contains(requestURN, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestURN == works[i]:
				workindex = i + 1
			case strings.Contains(requestURN, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestURN
		result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
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
			if RequestedWork.URN[i] == requestURN {
				requestedIndex = i
			}
		}
		switch {
		case contains(RequestedWork.URN, requestURN):
			switch {
			case requestedIndex == 0:
				result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: []Node{}}
			case requestedIndex-1 == 0:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex-1]},
						Text:  []string{RequestedWork.Text[requestedIndex-1]},
						Next:  []string{RequestedWork.URN[requestedIndex]},
						Index: RequestedWork.Index[requestedIndex-1]}}}
			default:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex-1]},
						Text:     []string{RequestedWork.Text[requestedIndex-1]},
						Next:     []string{RequestedWork.URN[requestedIndex]},
						Previous: []string{RequestedWork.URN[requestedIndex-2]},
						Index:    RequestedWork.Index[requestedIndex-1]}}}
			}
		default:
			message := "Could not find node to " + requestURN + " in source."
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		}
	}
	result.Service = "/texts/previous"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintln(w, string(resultJSON))
	log.Println("ReturnPrev executed succesfully")
}

func ReturnNext(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnNext")
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from config instead.")
	}
	requestURN := vars["URN"]
	if isCTSURN(requestURN) != true {
		message := requestURN + " is not valid CTS."
		result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		result.Service = "/texts/next"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		log.Println("ReturnReff executed succesfully")
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
		if strings.Contains(requestURN, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestURN == works[i]:
				workindex = i + 1
			case strings.Contains(requestURN, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestURN
		result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
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
			if RequestedWork.URN[i] == requestURN {
				requestedIndex = i
			}
		}
		switch {
		case contains(RequestedWork.URN, requestURN):
			switch {
			case requestedIndex == len(RequestedWork.URN)-1:
				result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: []Node{}}
			case requestedIndex+1 == len(RequestedWork.URN)-1:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex+1]},
						Text:     []string{RequestedWork.Text[requestedIndex+1]},
						Previous: []string{RequestedWork.URN[requestedIndex]},
						Index:    RequestedWork.Index[requestedIndex+1]}}}
			default:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex+1]},
						Text:     []string{RequestedWork.Text[requestedIndex+1]},
						Next:     []string{RequestedWork.URN[requestedIndex+2]},
						Previous: []string{RequestedWork.URN[requestedIndex]},
						Index:    RequestedWork.Index[requestedIndex+1]}}}
			}
		default:
			message := "Could not find node to " + requestURN + " in source."
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		}
	}
	result.Service = "/texts/next"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	log.Println("ReturnNext executed succesfully")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnReff(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnReff")
	confvar := LoadConfiguration("config.json") //load configuration from json file (ServerConfig)
	vars := mux.Vars(r)                         //load vars from mux config to get CEX and URN information )[]string ?)
	requestCEX := ""                            //initalize CEX variable (string)
	requestCEX = vars["CEX"]                    //save CEX name in CEX variable
	var sourcetext string                       //initialize sourcetext variable; will hold CEX data
	switch {                                    //switch to determine whether a CEX file was specified
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex" //build URL to CEX file if CEX file was specified
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource //use TestSource as URL to CEX-file as found in config.json
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from config instead.")
	}
	requestURN := vars["URN"]         //safe requested URN
	if isCTSURN(requestURN) != true { //test if given URN is valid (bool)
		message := requestURN + " is not valid CTS."                                                    //build message part of NodeResponse
		result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message} //building result (NodeResponse)
		result.Service = "/texts/urns"                                                                  // adding Service part to result (NodeResponse)
		resultJSON, _ := json.Marshal(result)                                                           //parsing result to JSON format (_ would contain err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")                               //set output format
		fmt.Fprintln(w, string(resultJSON))                                                             //output
		log.Println("ReturnReff executed succesfully")
		return
	}
	workResult := ParseWork(CTSParams{Sourcetext: sourcetext}) //parse the work
	works := append([]string(nil), workResult.URN...)          // append URNs from workResult to works
	for i := range workResult.URN {
		works[i] = strings.Join(strings.Split(workResult.URN[i], ":")[0:4], ":") //crop URNs in []work to first four parts of URN
	}
	works = removeDuplicatesUnordered(works) //remove dublicate URNS
	workindex := 0                           //initialize variable to save index of works to work with
	for i := range works {
		if strings.Contains(requestURN, works[i]) {
			teststring := works[i] + ":" //add colon which was lost during joins
			switch {
			case requestURN == works[i]:
				workindex = i + 1
			case strings.Contains(requestURN, teststring): //should have matched before already, shouldn't it?
				workindex = i + 1
			}
		}
	}
	var result URNResponse //initialize result (URNResponse)
	switch {
	case workindex == 0: //if requested URN is not among URNs in works prepare and display message accordingly
		message := "No results for " + requestURN
		result = URNResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
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
		case isRange(requestURN): //if range is requested,
			ctsurn := splitCTS(requestURN)                   //split URN into its stem and reference
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
			result = URNResponse{requestURN: []string{requestURN}, Status: "Success", URN: range_urn} //assemble result
			result.Service = "/texts/urns"
			resultJSON, _ := json.Marshal(result)                             //parse result to json format
			w.Header().Set("Content-Type", "application/json; charset=utf-8") //set output format
			fmt.Fprintln(w, string(resultJSON))                               //output
		default:
			switch {
			case contains(RequestedWork.URN, requestURN):
				result = URNResponse{requestURN: []string{requestURN}, Status: "Success", URN: []string{requestURN}}
			case level1contains(RequestedWork.URN, requestURN):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{requestURN: []string{requestURN}, Status: "Success", URN: matchingURNs}
			case level2contains(RequestedWork.URN, requestURN):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{requestURN: []string{requestURN}, Status: "Success", URN: matchingURNs}
			case level3contains(RequestedWork.URN, requestURN):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{requestURN: []string{requestURN}, Status: "Success", URN: matchingURNs}
			case level4contains(RequestedWork.URN, requestURN):
				var matchingURNs []string
				var match []bool
				for i := range RequestedWork.URN {
					match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
					match = append(match, match2)
				}
				for i := range match {
					if match[i] == true {
						matchingURNs = append(matchingURNs, RequestedWork.URN[i])
					}
				}
				result = URNResponse{requestURN: []string{requestURN}, Status: "Success", URN: matchingURNs}
			default:
				result = URNResponse{requestURN: []string{requestURN}, Status: "Exception", Message: "Couldn't find URN."}
			}
			result.Service = "/texts/urns"
			resultJSON, _ := json.Marshal(result)                             //parse result to json format
			w.Header().Set("Content-Type", "application/json; charset=utf-8") //set output format
			fmt.Fprintln(w, string(resultJSON))                               //output
			log.Println("ReturnReff executed succesfully")
		}
	}
}

//Returns a passage according to CEX file and URN specified
func ReturnPassage(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnPassage")
	confvar := LoadConfiguration("config.json")
	vars := mux.Vars(r)
	requestCEX := ""
	requestCEX = vars["CEX"]
	var sourcetext string
	switch {
	case requestCEX != "":
		sourcetext = confvar.Source + requestCEX + ".cex"
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from config instead.")
	}
	requestURN := vars["URN"]
	if isCTSURN(requestURN) != true {
		message := requestURN + " is not valid CTS."
		result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		result.Service = "/texts"
		resultJSON, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintln(w, string(resultJSON))
		log.Println("ReturnPassage executed succesfully")
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
		if strings.Contains(requestURN, works[i]) {
			teststring := works[i] + ":"
			switch {
			case requestURN == works[i]:
				workindex = i + 1
			case strings.Contains(requestURN, teststring):
				workindex = i + 1
			}
		}
	}
	var result NodeResponse
	switch {
	case workindex == 0:
		message := "No results for " + requestURN
		result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
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
			if RequestedWork.URN[i] == requestURN {
				requestedIndex = i
			}
		}
		switch {
		case contains(RequestedWork.URN, requestURN):
			switch {
			case requestedIndex == 0:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex]},
						Text:  []string{RequestedWork.Text[requestedIndex]},
						Next:  []string{RequestedWork.URN[requestedIndex+1]},
						Index: RequestedWork.Index[requestedIndex]}}}
			case requestedIndex == len(RequestedWork.URN)-1:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex]},
						Text:     []string{RequestedWork.Text[requestedIndex]},
						Previous: []string{RequestedWork.URN[requestedIndex-1]},
						Index:    RequestedWork.Index[requestedIndex]}}}
			default:
				result = NodeResponse{requestURN: []string{requestURN},
					Status: "Success",
					Nodes: []Node{Node{URN: []string{RequestedWork.URN[requestedIndex]},
						Text:     []string{RequestedWork.Text[requestedIndex]},
						Next:     []string{RequestedWork.URN[requestedIndex+1]},
						Previous: []string{RequestedWork.URN[requestedIndex-1]},
						Index:    RequestedWork.Index[requestedIndex]}}}
			}
		case level1contains(RequestedWork.URN, requestURN):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+)$"), RequestedWork.URN[i])
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
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: matchingNodes}
		case level2contains(RequestedWork.URN, requestURN):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
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
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: matchingNodes}
		case level3contains(RequestedWork.URN, requestURN):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
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
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: matchingNodes}
		case level4contains(RequestedWork.URN, requestURN):
			var matchingNodes []Node
			var match []bool
			for i := range RequestedWork.URN {
				match2, _ := regexp.MatchString((requestURN + "([:|.]*[0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+).([0-9|a-z]+)$"), RequestedWork.URN[i])
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
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: matchingNodes}
		case isRange(requestURN):
			var rangeNodes []Node
			ctsurn := splitCTS(requestURN)
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
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Success", Nodes: rangeNodes}
		default:
			message := "Could not find node to " + requestURN + " in source."
			result = NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message}
		}
	}
	result.Service = "/texts"
	resultJSON, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	log.Println("ReturnPassage executed succesfully")
	fmt.Fprintln(w, string(resultJSON))
}

func ReturnCatalog(w http.ResponseWriter, r *http.Request) {
	log.Println("Called function: ReturnCatalog")
	confvar := LoadConfiguration("config.json") //load configuration from json file (ServerConfig)
	vars := mux.Vars(r)                         //load vars from mux config to get CEX and URN information )[]string ?)
	requestCEX := ""                            //initalize CEX variable (string)
	requestCEX = vars["CEX"]                    //save CEX name in CEX variable
	var sourcetext string                       //initialize sourcetext variable; will hold CEX data
	switch {                                    //switch to determine whether a CEX file was specified
	case requestCEX != "": //either {CEX}/catalog/ or /{CEX}/catalog/{URN}
		sourcetext = confvar.Source + requestCEX + ".cex" //build URL to CEX file if CEX file was specified
		log.Println("CEX-file provided in URL: " + requestCEX + ". Using " + sourcetext + ".")
	default:
		sourcetext = confvar.TestSource //use TestSource as URL to CEX-file as found in config.json instead
		log.Println("No CEX-file provided in URL. Using " + confvar.TestSource + " from congfig instead.")
	}

	requestURN := ""         //initialize requestURN (string)
	requestURN = vars["URN"] //safe URN in variable

	defer func() {
		if catalogError := recover(); catalogError != nil {
			message := ("Error encountered. Please contact development team and send in current logfile!")  //build message part of NodeResponse
			result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message} //building result (NodeResponse)
			result.Service = "/catalog"                                                                     // adding Service part to result (NodeResponse)
			resultJSON, _ := json.Marshal(result)                                                           //parsing result to JSON format (_ would contain err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")                               //set output format
			fmt.Fprintln(w, string(resultJSON))
			log.Println("Error encountered: \"", catalogError, "\"")

		}
	}()

	switch {
	case requestURN != "": //if the request URN was specified (not empty)
		requestURN = strings.Join(strings.Split(requestURN, ":")[0:4], ":") //crop URN to first 4 parts (passage not needed for catalog
		requestURN = (requestURN + ":")                                     //add ":" in the end to match appearance in catalog.

		if isCTSURN(requestURN) != true { //test if given URN is valid (bool), if not give an error message
			message := requestURN + " is not valid CTS."                                                    //build message part of NodeResponse
			result := NodeResponse{requestURN: []string{requestURN}, Status: "Exception", Message: message} //building result (NodeResponse)
			result.Service = "/catalog"                                                                     // adding Service part to result (NodeResponse)
			resultJSON, _ := json.Marshal(result)                                                           //parsing result to JSON format (_ would contain err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")                               //set output format
			fmt.Fprintln(w, string(resultJSON))                                                             //output
			log.Println("ReturnCatalog executed succesfully")
			return
		}

		catalogResult := ParseCatalog(CTSParams{Sourcetext: sourcetext}) //parse the catalog
		//ToDo: check if catalogResult is empty --> Message + log
		entries := catalogResult.CatalogEntries // get Catalog Entries ([]CatalogEntry)
		var urns []string                       // create array to hold urns
		for i := range entries {
			urns = append(urns, entries[i].URN)
		}
		urns = removeDuplicatesUnordered(urns)
		switch {
		case contains(urns, requestURN):
			message := requestURN + " is in the CTS Catalog."
			result := CatalogResponse{Status: "Success", Message: message}
			result.Service = "/catalog"
			resultJSON, _ := json.Marshal(result)
			w.Header().Set("Content-Type", "application/json; charset=utf-8") //set output format
			fmt.Fprintln(w, string(resultJSON))                               //output
			log.Println("ReturnCatalog executed succesfully")
			return
		default:
			message := requestURN + " is not in the CTS Catalog. Printing URNs in catalog" //build message part of CatalogResponse
			result := CatalogResponse{Status: "Exception", Message: message, URN: urns}    //building result (CataloResponse)
			result.Service = "/catalog"                                                    //adding Service part to result (NodeResponse)
			resultJSON, _ := json.Marshal(result)                                          //parsing result to JSON format (_ would contain err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")              //set output format
			fmt.Fprintln(w, string(resultJSON))                                            //output
			log.Println("ReturnCatalog executed succesfully")
			return
		}
	default:
		catalogResult := ParseCatalog(CTSParams{Sourcetext: sourcetext}) //parse the catalog
		entries := catalogResult.CatalogEntries                          // get Catalog Entries ([]CatalogEntry)
		var urns []string                                                // create string to hold urns
		for i := range entries {
			urns = append(urns, entries[i].URN)
		}
		urns = removeDuplicatesUnordered(urns)

		message := "No URN specified. Printing URNs in catalog"                   //build message part of CatalogResponse
		result := CatalogResponse{Status: "Success", Message: message, URN: urns} //building result (CataloResponse)
		result.Service = "/catalog"                                               //adding Service part to result (NodeResponse)
		resultJSON, _ := json.Marshal(result)                                     //parsing result to JSON format (_ would contain err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")         //set output format
		fmt.Fprintln(w, string(resultJSON))                                       //output
		log.Println("ReturnCatalog executed succesfully")
	}
}
