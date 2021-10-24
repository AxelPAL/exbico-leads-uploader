package main

type recordProcessingElement struct {
	Record []string
	Lead   Lead
}
type recordProcessingResult struct {
	Record []string
	Lead   Lead
	Status string
	Data   LeadSendingResponseData
}
