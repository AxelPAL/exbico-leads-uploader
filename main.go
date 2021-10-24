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
const MaxThreadsCount = 10

var apiUrl string
var debugMode bool
var leadFilePath string
var outputFileName string
var threads int
var token string

func main() {
	if threads > MaxThreadsCount {
		log.Fatal(fmt.Sprintf("Количество потоков должно быть не больше %d.", MaxThreadsCount))
	}
	records, err := readData(leadFilePath)
	if err != nil {
		if debugMode {
			log.Println(err)
		}
		log.Fatal("Файл с лидами имеет неправильный формат. Он должен быть в формате csv с разделителем `,`")
	}
	fileLinesCount, err := calcCsvFileLinesCount(leadFilePath)
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

	exitProgram()
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
		status, response := sendLead(recordProcessingElement.Lead, token)
		asyncResult := recordProcessingResult{
			Record:  recordProcessingElement.Record,
			Lead:    recordProcessingElement.Lead,
			Status:  status,
			Data:    response.Data,
			Message: response.Message,
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
		leadStatus := recordProcessingResult.Data.LeadStatus
		rejectReason := recordProcessingResult.Data.RejectReason
		leadId := recordProcessingResult.Data.LeadId
		var leadIdString string
		if leadId > 0 {
			leadIdString = strconv.Itoa(recordProcessingResult.Data.LeadId)
		}
		err := writeResultCsv(
			recordProcessingResult.Record,
			translateResponseStatus(recordProcessingResult.Status),
			translateLeadStatus(leadStatus),
			translateRejectionReason(rejectReason),
			leadIdString,
			recordProcessingResult.Message,
		)
		if err != nil {
			if debugMode {
				log.Println(err)
			}
		}
		bar.Increment()
		return true
	})
	bar.Finish()
	fmt.Println("Результат сохранён в файл " + outputFileName)
}

func translateResponseStatus(status string) string {
	dict := map[string]string{
		"success": "Успех",
		"fail":    "Ошибка данных",
		"error":   "Ошибка сервера",
	}

	return applyTranslation(dict, status)
}

func translateLeadStatus(leadStatus string) string {
	dict := map[string]string{
		"inProgress": "Принят",
		"rejected":   "Не принят",
	}

	return applyTranslation(dict, leadStatus)
}

func translateRejectionReason(rejectionReason string) string {
	dict := map[string]string{
		"isDouble": "Дубль",
	}

	return applyTranslation(dict, rejectionReason)
}

func applyTranslation(dictMap map[string]string, valueToTranslate string) string {
	result := valueToTranslate
	value, exists := dictMap[valueToTranslate]
	if exists {
		result = value
	}

	return result
}

func writeHeadLineIntoOutputFile() {
	headLine := []string{"Фамилия", "Имя", "Отчество", "Дата рождения", "Возраст", "Телефон", "E-mail", "Сумма кредита", "Срок кредита", "Регион", "Город", "Серия паспорта", "Номер паспорта", "Дата выдачи паспорта"}
	err := writeResultCsv(headLine, "Результат отправки", "Лид принят", "Причина отбраковки лида", "ID лида", "Дополнительная информация по приёму лида")
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

func writeResultCsv(record []string, leadSendingResult string, leadStatus string, rejectionReason string, leadId string, leadErrorsString string) error {
	file, err := os.OpenFile(outputFileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	checkError("Cannot create file", err)
	record = append(record, leadSendingResult, leadStatus, rejectionReason, leadId, leadErrorsString)
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
	if token == "" {
		tokenFromFile, err := getToken()
		if err != nil {
			log.Fatal(err)
		}
		token = tokenFromFile
	}
	if len(token) != 32 {
		log.Fatal("Токен должен содержать ровно 32 символа (в файле token.txt)")
	}
}

func initFlags() {
	apiUrlPointer := flag.String("apiUrl", ExbicoLeadApiUrl, "url of Exbico Lead Api")
	debugModePointer := flag.Bool("debug", false, "enable debug mode")
	threadsPointer := flag.Int("threads", 2, fmt.Sprintf("number of parallel threads (max=%d)", MaxThreadsCount))
	leadFilePathPointer := flag.String("leadFilePath", FileWithLeadsName, "path to csv-file with leads")
	tokenPointer := flag.String("token", "", "token to Exbico Leads API")
	flag.Parse()
	apiUrl = *apiUrlPointer
	debugMode = *debugModePointer
	threads = *threadsPointer
	leadFilePath = *leadFilePathPointer
	token = *tokenPointer
}

func formatDate(date string) string {
	var t *goment.Goment
	t, _ = goment.New(date)
	if t.ToUnix() < 0 {
		t, _ = goment.New(date, "DD.MM.YYYY")
	}
	return t.Format("YYYY-MM-DD")
}

func sendLead(lead Lead, token string) (string, LeadSendingResponse) {
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

	return parseResponseBody(body, resp.StatusCode)
}

func parseResponseBody(body []byte, statusCode int) (string, LeadSendingResponse) {
	response := LeadSendingResponse{}

	if statusCode == 200 {
		err := json.Unmarshal(body, &response)
		if err != nil && debugMode {
			fmt.Println(err)
		}
	} else {
		response.Status = "error"
	}
	return response.Status, response
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

func exitProgram() {
	fmt.Println("Нажмите клавишу Enter для завершения работы программы...")
	_, _ = fmt.Scanln()
}
