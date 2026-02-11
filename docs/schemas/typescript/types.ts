// PassBi Core API TypeScript Types
// Generated from OpenAPI specification

export type TransitMode = 'BUS' | 'BRT' | 'TER' | 'FERRY' | 'TRAM';
export type StepType = 'WALK' | 'RIDE' | 'TRANSFER';
export type RoutingStrategy = 'no_transfer' | 'direct' | 'simple' | 'fast';

export interface RouteSearchResponse {
  routes: {
    no_transfer?: RouteResult;
    direct?: RouteResult;
    simple?: RouteResult;
    fast?: RouteResult;
  };
}

export interface RouteResult {
  duration_seconds: number;
  walk_distance_meters: number;
  transfers: number;
  steps: Step[];
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
}

export interface NearbyStopsResponse {
  stops: NearbyStop[];
}

export interface NearbyStop {
  id: string;
  name: string;
  lat: number;
  lon: number;
  distance_meters: number;
  routes: string[];
  routes_count: number;
}

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
