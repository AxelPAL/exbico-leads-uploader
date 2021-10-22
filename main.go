package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	"github.com/clarketm/json"
	"github.com/google/uuid"
	"github.com/nleeper/goment"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const ExbicoLeadApiUrl = "https://app.exbico.ru/api/leads/supplier/v1/credit-lead"
const FileWithLeadsName = "leads.csv"

var apiUrl string
var debugMode bool
var outputFileName string
var threads int
var token string

type recordProcessingElement struct {
	Record []string
	Lead   Lead
}
type recordProcessingResult struct {
	Record      []string
	Lead        Lead
	Result      string
	ErrorString string
}

func main() {
	if threads > 10 {
		log.Fatal("Количество потоков должно быть не больше 10.")
	}
	records, err := readData(FileWithLeadsName)
	if err != nil {
		log.Fatal(err)
	}
	fileLinesCount, err := calcCsvFileLinesCount(FileWithLeadsName)
	bar := pb.StartNew(fileLinesCount)
	setOutputFileName()
	writeHeadLineIntoOutputFile()

	jobs := new(sync.Map)
	results := new(sync.Map)
	wg := new(sync.WaitGroup)

	for _, record := range records {
		addLeadToMap(record, jobs)
	}

	fmt.Println("Отправка данных...")
	maxHashMapLengthForWorker := len(records) / threads
	chunkedHashMap := make(map[string]recordProcessingElement)
	jobs.Range(func(k, v interface{}) bool {
		element := v.(recordProcessingElement)
		chunkedHashMap[fmt.Sprintf("%s", k)] = element
		if len(chunkedHashMap) == maxHashMapLengthForWorker {
			clonedHashMap := make(map[string]recordProcessingElement)
			for key, value := range chunkedHashMap {
				clonedHashMap[key] = value
				delete(chunkedHashMap, key)
			}
			wg.Add(1)
			go worker(clonedHashMap, token, results, wg, bar)
		}
		return true
	})
	if len(chunkedHashMap) > 0 {
		wg.Add(1)
		go worker(chunkedHashMap, token, results, wg, bar)
	}

	wg.Wait()
	bar.Finish()
	writeResults(fileLinesCount, results)
	fileLinesCount, err = calcCsvFileLinesCount(outputFileName)
}

func addLeadToMap(record []string, jobs *sync.Map) {
	var uuidString uuid.UUID
	uuidString, _ = uuid.NewRandom()
	lead := prepareLead(record)
	recordProcessingElement := recordProcessingElement{
		Record: record,
		Lead:   lead,
	}
	jobs.Store(uuidString, recordProcessingElement)
}

func worker(hashMap map[string]recordProcessingElement, token string, results *sync.Map, wg *sync.WaitGroup, bar *pb.ProgressBar) {
	defer wg.Done()
	for key, recordProcessingElement := range hashMap {
		if debugMode {
			leadJson, _ := json.Marshal(recordProcessingElement.Lead)
			fmt.Println(string(leadJson))
		}
		errorString, result := sendLead(recordProcessingElement.Lead, token)
		asyncResult := recordProcessingResult{
			Record:      recordProcessingElement.Record,
			Lead:        recordProcessingElement.Lead,
			Result:      result,
			ErrorString: errorString,
		}
		results.Store(key, asyncResult)
		bar.Increment()
	}
}

func writeResults(fileLinesCount int, results *sync.Map) {
	fmt.Println("Сохранение результата...")
	bar := pb.StartNew(fileLinesCount)
	results.Range(func(k, v interface{}) bool {
		recordProcessingResult := v.(recordProcessingResult)
		err := writeResultCsv(recordProcessingResult.Record, recordProcessingResult.ErrorString, recordProcessingResult.Result)
		if err != nil {
			if debugMode {
				log.Println(err)
			}
		}
		bar.Increment()
		return true
	})
	bar.Finish()
}

func writeHeadLineIntoOutputFile() {
	headLine := []string{"Фамилия", "Имя", "Отчество", "Дата рождения", "Возраст", "Телефон", "E-mail", "Сумма кредита", "Срок кредита", "Регион", "Город", "Серия паспорта", "Номер паспорта", "Дата выдачи паспорта"}
	err := writeResultCsv(headLine, "Ошибка в данных лида", "Результат отправки")
	if err != nil {
		if debugMode {
			log.Println(err)
		}
	}
}

func setOutputFileName() {
	if outputFileName == "" {
		outputFileName = fmt.Sprintf("result_%s.csv", time.Now().Format("2006-01-02_15_04_05"))
	}
}

func writeResultCsv(record []string, errorString string, leadSendingResult string) error {
	file, err := os.OpenFile(outputFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	checkError("Cannot create file", err)
	record = append(record, leadSendingResult, errorString)
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write(record)
	if err != nil {
		return err
	}
	return nil
}

func checkError(message string, err error) {
	if err != nil {
		log.Fatal(message, err)
	}
}

func prepareLead(record []string) Lead {
	var lead = Lead{}
	lead.Client.FirstName = record[0]
	lead.Client.LastName = record[1]
	lead.Client.Patronymic = record[2]
	if record[3] != "" {
		lead.Client.BirthDate = formatDate(record[3])
	}
	age, _ := strconv.Atoi(record[4])
	lead.Client.Age = age
	lead.Client.Phone = record[5]
	lead.Client.Email = record[6]
	lead.Product.TypeId = "consumer"
	amount, _ := strconv.Atoi(record[7])
	lead.Product.Amount = amount
	lead.Product.Term = record[8]
	lead.Location.Name.Region = record[9]
	lead.Location.Name.City = record[10]
	lead.Passport.Series = record[11]
	lead.Passport.Number = record[12]
	if record[13] != "" {
		lead.Passport.IssueDate = formatDate(record[13])
	}

	return lead
}

func init() {
	initFlags()
	initToken()
}

func initToken() {
	tokenFromFile, err := getToken()
	if err != nil {
		log.Fatal(err)
	}
	token = tokenFromFile
	if len(token) != 32 {
		log.Fatal("Токен должен содержать ровно 32 символа (в файле token.txt)")
	}
}

func initFlags() {
	apiUrlPointer := flag.String("apiUrl", ExbicoLeadApiUrl, "url of Exbico Lead Api")
	debugModePointer := flag.Bool("debug", false, "enable debug mode")
	threadsPointer := flag.Int("threads", 2, "number of parallel threads (max=50)")
	flag.Parse()
	apiUrl = *apiUrlPointer
	debugMode = *debugModePointer
	threads = *threadsPointer
}

func formatDate(date string) string {
	var t *goment.Goment
	t, _ = goment.New(date)
	if t.ToUnix() < 0 {
		t, _ = goment.New(date, "DD.MM.YYYY")
	}
	return t.Format("YYYY-MM-DD")
}

func sendLead(lead Lead, token string) (string, string) {
	leadJson, _ := json.Marshal(lead)
	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(leadJson))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tool-Version", "v1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	body, _ := ioutil.ReadAll(resp.Body)
	if debugMode {
		fmt.Println("request Url:", req.URL)
		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)
		fmt.Println("response Body:", string(body))
	}

	return parseResponseBody(body)
}

func parseResponseBody(body []byte) (string, string) {
	response := LeadSendingResponse{}

	err := json.Unmarshal(body, &response)
	if err != nil {
		fmt.Println(err)
	}
	return response.Data.LeadStatus, response.Message
}

func readData(fileName string) ([][]string, error) {

	f, err := os.Open(fileName)
	if err != nil {
		return [][]string{}, err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)
	r := csv.NewReader(f)
	// skip first line
	if _, err := r.Read(); err != nil {
		return [][]string{}, err
	}
	records, err := r.ReadAll()
	if err != nil {
		return [][]string{}, err
	}

	return records, nil
}

func getToken() (string, error) {
	tokenFile, err := os.Open("token.txt")
	if err != nil {
		log.Fatal(err)
	}
	var token string
	scanner := bufio.NewScanner(tokenFile)
	for scanner.Scan() {
		token = scanner.Text()
	}

	return token, err
}

func calcCsvFileLinesCount(fileName string) (int, error) {
	r, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(r)
	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}
