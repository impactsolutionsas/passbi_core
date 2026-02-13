// PassBi Core API TypeScript Types
// Generated from OpenAPI specification

export type TransitMode = 'BUS' | 'BRT' | 'TER' | 'FERRY' | 'TRAM';
export type StepType = 'WALK' | 'RIDE' | 'TRANSFER';
export type RoutingStrategy = 'no_transfer' | 'direct' | 'simple' | 'fast';

// --- Route Search ---

export interface RouteSearchResponse {
  routes: {
    no_transfer?: RouteResult;
    direct?: RouteResult;
    simple?: RouteResult;
    fast?: RouteResult;
  };
  departure_time: string;
}

export interface RouteResult {
  duration_seconds: number;
  walk_distance_meters: number;
  transfers: number;
  arrival_time: string;
  steps: Step[];
}

export interface StopInfo {
  id: string;
  name: string;
}

export interface Step {
  type: StepType;
  from_stop: string;
  to_stop: string;
  from_stop_name: string;
  to_stop_name: string;
  route?: string;
  route_name?: string;
  mode?: TransitMode;
  duration_seconds: number;
  distance_meters?: number;
  num_stops?: number;
  stops?: StopInfo[];
  departure_time?: string;
  arrival_time?: string;
  agency_name?: string;
}

// --- Nearby Stops ---

export interface NearbyStopsResponse {
  stops: NearbyStop[];
}

export interface NearbyRouteInfo {
  id: string;
  name: string;
  mode: TransitMode;
  agency_id: string;
  agency_name: string;
}

export interface NearbyStop {
  id: string;
  name: string;
  lat: number;
  lon: number;
  distance_meters: number;
  modes: TransitMode[];
  routes: NearbyRouteInfo[];
  routes_count: number;
}

// --- Routes List ---

export interface RoutesListResponse {
  routes: RouteInfo[];
  total: number;
}

export interface RouteInfo {
  id: string;
  name: string;
  mode: TransitMode;
  agency_id: string;
  stops_count: number;
}

// --- Stop Search ---

export interface StopSearchResponse {
  stops: StopSearchResult[];
  query: string;
  total: number;
}

export interface StopSearchResult {
  id: string;
  name: string;
  lat: number;
  lon: number;
}

// --- Departures ---

export interface DeparturesResponse {
  stop: StopBasic;
  departures: DepartureInfo[];
  current_time: string;
  date: string;
  total: number;
}

export interface StopBasic {
  id: string;
  name: string;
  lat: number;
  lon: number;
}

export interface DepartureInfo {
  route_id: string;
  route_name: string;
  mode: TransitMode;
  agency_id: string;
  agency_name: string;
  headsign: string;
  direction: number;
  departure_time: string;
  departure_seconds: number;
  minutes_until: number;
  trip_id: string;
  service_id: string;
  service_active: boolean;
}

// --- Schedule ---

export interface ScheduleResponse {
  route: RouteBasic;
  services: ScheduleService[];
  stops: ScheduleStop[];
  trips: ScheduleTrip[];
  total_trips: number;
}

export interface RouteBasic {
  id: string;
  name: string;
  mode: TransitMode;
  agency_id: string;
}

export interface ScheduleService {
  service_id: string;
  days: string[];
  start_date?: string;
  end_date?: string;
}

export interface ScheduleStop {
  id: string;
  name: string;
  sequence: number;
}

export interface ScheduleTrip {
  trip_id: string;
  service_id: string;
  headsign: string;
  direction: number;
  times: string[];
}

// --- Trips ---

export interface TripsResponse {
  route: RouteBasic;
  trips: TripDetail[];
  total: number;
  limit: number;
  offset: number;
}

export interface TripDetail {
  trip_id: string;
  service_id: string;
  headsign: string;
  direction: number;
  stops: TripStopTime[];
}

export interface TripStopTime {
  stop_id: string;
  stop_name: string;
  sequence: number;
  arrival_time: string;
  departure_time: string;
}

// --- System ---

export interface HealthResponse {
  status: 'healthy' | 'unhealthy';
  checks: {
    database: string;
    redis: string;
  };
}

export interface ErrorResponse {
  error: string;
}
