package models

import "time"

// TransitMode represents the type of transit service
type TransitMode string

const (
	ModeBus   TransitMode = "BUS"
	ModeBRT   TransitMode = "BRT"
	ModeTER   TransitMode = "TER"
	ModeFerry TransitMode = "FERRY"
	ModeTram  TransitMode = "TRAM"
)

// EdgeType represents the type of connection between nodes
type EdgeType string

const (
	EdgeWalk     EdgeType = "WALK"
	EdgeRide     EdgeType = "RIDE"
	EdgeTransfer EdgeType = "TRANSFER"
)

// Stop represents a physical transit stop location
type Stop struct {
	ID        string
	Name      string
	Lat       float64
	Lon       float64
	CreatedAt time.Time
}

// Route represents a transit route (line)
type Route struct {
	ID        string
	AgencyID  string
	ShortName string
	LongName  string
	Mode      TransitMode
	CreatedAt time.Time
}

// Node represents a (stop, route) pair in the routing graph
// Each node is a unique combination of a stop and a route serving that stop
type Node struct {
	ID        int64
	StopID    string
	StopName  string
	RouteID   string
	RouteName string
	Mode      TransitMode
	Lat       float64
	Lon       float64
	CreatedAt time.Time
}

// Edge represents a connection between two nodes in the routing graph
type Edge struct {
	ID           int64
	FromNodeID   int64
	ToNodeID     int64
	Type         EdgeType
	CostTime     int // seconds
	CostWalk     int // meters
	CostTransfer int // count (0 or 1)
	TripID       string
	Sequence     int
	CreatedAt    time.Time
}

// Path represents a complete route from origin to destination
type Path struct {
	Nodes         []Node
	Edges         []Edge
	TotalTime     int // seconds
	TotalWalk     int // meters
	Transfers     int // count
	Strategy      string
	DurationMins  int
	WalkDistanceM int
	Steps         []Step
}

// StopInfo represents a stop in a journey step
type StopInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Step represents one segment of a journey
type Step struct {
	Type         EdgeType    `json:"type"`
	FromStop     string      `json:"from_stop"`
	ToStop       string      `json:"to_stop"`
	FromStopName string      `json:"from_stop_name"`
	ToStopName   string      `json:"to_stop_name"`
	Route        string      `json:"route,omitempty"`
	RouteName    string      `json:"route_name,omitempty"`
	Mode         TransitMode `json:"mode,omitempty"`
	Duration     int         `json:"duration_seconds"`
	Distance     int         `json:"distance_meters,omitempty"`
	NumStops     int         `json:"num_stops,omitempty"`
	Stops        []StopInfo  `json:"stops,omitempty"` // Intermediate stops for RIDE steps
}

// GTFS data structures for import

// GTFSAgency represents an agency from agency.txt
type GTFSAgency struct {
	AgencyID   string
	AgencyName string
	AgencyURL  string
	Timezone   string
}

// GTFSStop represents a stop from stops.txt
type GTFSStop struct {
	StopID   string
	StopName string
	Lat      float64
	Lon      float64
}

// GTFSRoute represents a route from routes.txt
type GTFSRoute struct {
	RouteID    string
	AgencyID   string
	ShortName  string
	LongName   string
	RouteType  int
	RouteColor string
}

// GTFSTrip represents a trip from trips.txt
type GTFSTrip struct {
	RouteID   string
	ServiceID string
	TripID    string
	Headsign  string
	Direction int
}

// GTFSStopTime represents a stop time from stop_times.txt
type GTFSStopTime struct {
	TripID       string
	ArrivalTime  string
	DepartureTime string
	StopID       string
	StopSequence int
}

// ImportLog represents a GTFS import operation log
type ImportLog struct {
	ID          int64
	AgencyID    string
	StartedAt   time.Time
	CompletedAt *time.Time
	Status      string
	StopsCount  int
	RoutesCount int
	NodesCount  int
	EdgesCount  int
	ErrorMsg    string
}
