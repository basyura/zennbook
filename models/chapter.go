package models

type Chapter struct {
	ID   int    `json:"id"`
	Name string `json:"title"`
	Url  string
}
