package audit

func auditSearchResult(page Page, err error) (Page, error) {
	if err != nil {
		return Page{Events: []Event{}}, nil
	}
	return page, nil
}
