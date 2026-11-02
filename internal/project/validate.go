package project

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

const (
	minNameLen        = 3
	maxNameLen        = 80
	maxLocationLen    = 120
	maxDescriptionLen = 500
	maxBusinessLen    = 60
	maxListItemLen    = 60
	maxCategories     = 40
	maxPhases         = 60
	// Thirteen digits of taka, as many as the budget field accepts.
	maxBudget = 9_999_999_999_999
)

// NewProject is a validated create request. Categories and phases arrive
// already worded in the creator's language: the new-project page shows them
// in its preview, so what the owner saw is what gets saved.
type NewProject struct {
	Name         string
	Kind         string
	Ownership    string
	Location     *string
	Description  *string
	StartDate    time.Time
	Budget       *int64
	FarmType     *string
	Floors       *int16
	Batches      *int16
	Plots        *int16
	LandArea     *float64
	LandUnit     *string
	BusinessType *string
	Categories   []string
	Phases       []string
}

type createBody struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Ownership    string   `json:"ownership"`
	Location     string   `json:"location"`
	Description  string   `json:"description"`
	StartDate    string   `json:"startDate"`
	Budget       *int64   `json:"budget"`
	FarmType     string   `json:"farmType"`
	Floors       *int16   `json:"floors"`
	Batches      *int16   `json:"batches"`
	Plots        *int16   `json:"plots"`
	LandArea     *float64 `json:"landArea"`
	LandUnit     string   `json:"landUnit"`
	BusinessType string   `json:"businessType"`
	Categories   []string `json:"categories"`
	Phases       []string `json:"phases"`
}

// validator collects field errors in the request's language.
type validator struct {
	lang   string
	fields []apperr.FieldError
}

func (v *validator) add(path, key string) {
	v.fields = append(v.fields, apperr.FieldError{Path: path, Message: fieldMessages[key].In(v.lang)})
}

// text trims s and returns nil for blank, flagging it when longer than limit.
func (v *validator) text(path, s string, limit int) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if utf8.RuneCountInString(s) > limit {
		v.add(path, path)
	}
	return &s
}

// count keeps n only for the kind it belongs to, and checks its range there.
func (v *validator) count(path string, n *int16, applies bool, limit int16) *int16 {
	if !applies || n == nil {
		return nil
	}
	if *n < 0 || *n > limit {
		v.add(path, "count")
	}
	return n
}

// list trims every entry and rejects blanks, repeats and over-long names.
func (v *validator) list(path string, items []string, limit int) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || utf8.RuneCountInString(item) > maxListItemLen || slices.Contains(out, item) {
			v.add(path, "list")
			return nil
		}
		out = append(out, item)
	}
	if len(out) > limit {
		v.add(path, "list")
	}
	return out
}

func (b createBody) validate(lang string) (NewProject, error) {
	v := &validator{lang: lang}
	np := NewProject{Name: strings.TrimSpace(b.Name), Kind: b.Kind, Ownership: b.Ownership}

	switch n := utf8.RuneCountInString(np.Name); {
	case n == 0:
		v.add("name", "nameMissing")
	case n < minNameLen:
		v.add("name", "nameShort")
	case n > maxNameLen:
		v.add("name", "nameLong")
	}
	if !slices.Contains(kinds, np.Kind) {
		v.add("kind", "kind")
	}
	if !slices.Contains(ownerships, np.Ownership) {
		v.add("ownership", "ownership")
	}
	np.Location = v.text("location", b.Location, maxLocationLen)
	np.Description = v.text("description", b.Description, maxDescriptionLen)
	start, err := time.Parse(time.DateOnly, b.StartDate)
	if err != nil {
		v.add("startDate", "startDate")
	}
	np.StartDate = start
	if b.Budget != nil && (*b.Budget < 0 || *b.Budget > maxBudget) {
		v.add("budget", "budget")
	}
	np.Budget = b.Budget

	np.applyKindDetails(v, b)
	np.Categories = v.list("categories", b.Categories, maxCategories)
	np.Phases = v.list("phases", b.Phases, maxPhases)

	if len(v.fields) > 0 {
		return NewProject{}, messages.Fail(apperr.KindInvalid, codeInvalidInput, v.fields...)
	}
	return np, nil
}

// applyKindDetails keeps only the details that belong to the chosen kind, so a
// building never carries a farm type left over from an earlier choice.
func (np *NewProject) applyKindDetails(v *validator, b createBody) {
	if np.Kind == KindFarm {
		if !slices.Contains(farmTypes, b.FarmType) {
			v.add("farmType", "farmType")
		}
		np.FarmType = &b.FarmType
	}
	np.Floors = v.count("floors", b.Floors, np.Kind == KindBuilding, 30)
	np.Batches = v.count("batches", b.Batches, np.Kind == KindFarm, 20)
	np.Plots = v.count("plots", b.Plots, np.Kind == KindLand, 50)
	if np.Kind == KindLand {
		if b.LandArea != nil && *b.LandArea < 0 {
			v.add("landArea", "landArea")
		}
		np.LandArea = b.LandArea
		if b.LandUnit != "" {
			if !slices.Contains(landUnits, b.LandUnit) {
				v.add("landUnit", "landUnit")
			}
			np.LandUnit = &b.LandUnit
		}
	}
	if np.Kind == KindBusiness {
		np.BusinessType = v.text("businessType", b.BusinessType, maxBusinessLen)
	}
}
