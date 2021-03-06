package main

type LeadSendingResponseData struct {
	LeadStatus   string `json:"leadStatus"`
	RejectReason string `json:"rejectReason"`
	LeadId       int    `json:"leadId"`
}
type LeadSendingResponse struct {
	Status  string                  `json:"status"`
	Message string                  `json:"message"`
	Version string                  `json:"version"`
	Data    LeadSendingResponseData `json:"data"`
	Code    int                     `json:"code"`
}
