// Package project manages projects (one set of books each) and who may open them.
package project

import "time"

// Project kinds, ownership, statuses and member roles, as stored.
const (
	KindBuilding   = "BUILDING"
	KindFarm       = "FARM"
	KindLand       = "LAND"
	KindBusiness   = "BUSINESS"
	KindRenovation = "RENOVATION"
	KindOther      = "OTHER"

	OwnershipPersonal    = "PERSONAL"
	OwnershipPartnership = "PARTNERSHIP"

	StatusActive = "ACTIVE"
	StatusOnHold = "ON_HOLD"
	StatusDone   = "DONE"

	RoleOwner   = "OWNER"
	RoleManager = "MANAGER"
	RoleViewer  = "VIEWER"
)

var (
	kinds      = []string{KindBuilding, KindFarm, KindLand, KindBusiness, KindRenovation, KindOther}
	ownerships = []string{OwnershipPersonal, OwnershipPartnership}
	farmTypes  = []string{"POULTRY", "CATTLE", "FISH", "CROP"}
	landUnits  = []string{"SHOTANGSHO", "KATHA", "BIGHA"}
)

// RestoreWindow is how long a deleted project can still be brought back.
const RestoreWindow = 30 * 24 * time.Hour

// Summary is one dashboard card: the project and the caller's place in it.
type Summary struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Ownership    string    `json:"ownership"`
	Status       string    `json:"status"`
	Location     *string   `json:"location"`
	Budget       *int64    `json:"budget"`
	Spent        int64     `json:"spent"`
	FarmType     *string   `json:"farmType"`
	Floors       *int16    `json:"floors"`
	BusinessType *string   `json:"businessType"`
	Role         string    `json:"role"`
	MemberCount  int       `json:"memberCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Project is the full record behind the overview page.
type Project struct {
	Summary
	Description  *string  `json:"description"`
	StartDate    string   `json:"startDate"`
	Batches      *int16   `json:"batches"`
	Plots        *int16   `json:"plots"`
	LandArea     *float64 `json:"landArea"`
	LandUnit     *string  `json:"landUnit"`
	Phases       []Phase  `json:"phases"`
	AccountCount int      `json:"accountCount"`
}

// Phase is a stage of the work with an optional budget of its own.
type Phase struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Budget *int64 `json:"budget"`
	Spent  int64  `json:"spent"`
}

// Deleted is a project in the owner's restore list.
type Deleted struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Ownership    string    `json:"ownership"`
	FarmType     *string   `json:"farmType"`
	BusinessType *string   `json:"businessType"`
	DeletedAt    time.Time `json:"deletedAt"`
	PurgeAt      time.Time `json:"purgeAt"`
	// Everyone but the owner, who lose access while it is deleted.
	Members []Member `json:"members"`
}

// Member is a person on a project and their role there.
type Member struct {
	Name string `json:"name"`
	Role string `json:"role"`
}
