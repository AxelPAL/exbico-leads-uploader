package main

type Lead struct {
	Product struct {
		TypeId string `json:"typeId,omitempty"`
		Amount int    `json:"amount,omitempty"`
		Term   string `json:"term,omitempty"`
		//BidId  string `json:"bidId,omitempty"`
	} `json:"product,omitempty"`
	Location struct {
		Name struct {
			Region string `json:"region,omitempty"`
			City   string `json:"city,omitempty"`
		} `json:"name,omitempty"`
		//Exbico struct {
		//	RegionId string `json:"regionId,omitempty"`
		//	CityId   string `json:"cityId,omitempty"`
		//} `json:"exbico,omitempty"`
		//Coordinates struct {
		//	Latitude  float64 `json:"latitude,omitempty"`
		//	Longitude float64 `json:"longitude,omitempty"`
		//} `json:"coordinates,omitempty"`
		//KladrId string `json:"kladrId,omitempty"`
	} `json:"location,omitempty"`
	Passport struct {
		Series    string `json:"series,omitempty"`
		Number    string `json:"number,omitempty"`
		IssueDate string `json:"issueDate,omitempty"`
	} `json:"passport,omitempty"`
	Client struct {
		FirstName  string `json:"firstName,omitempty"`
		LastName   string `json:"lastName,omitempty"`
		Patronymic string `json:"patronymic,omitempty"`
		BirthDate  string `json:"birthDate,omitempty"`
		Age        int    `json:"age,omitempty"`
		Phone      string `json:"phone,omitempty"`
		Email      string `json:"email,omitempty"`
	} `json:"client,omitempty"`
	//Meta struct {
	//	UtmSource   string `json:"utmSource,omitempty"`
	//	UtmMedium   string `json:"utmMedium,omitempty"`
	//	UtmCampaign string `json:"utmCampaign,omitempty"`
	//	UtmTerm     string `json:"utmTerm,omitempty"`
	//	UtmContent  string `json:"utmContent,omitempty"`
	//} `json:"meta,omitempty"`
	//AgreedWithPersonalDataTransfer bool `json:"agreedWithPersonalDataTransfer,omitempty"`
	//ConfirmedWithSms               bool `json:"confirmedWithSms,omitempty"`
}
