# Exemples Ionic/Angular

Intégration complète de PassBi API dans une application Ionic/Angular.

## Installation

```bash
npm install @capacitor/core @capacitor/geolocation
# ou pour une nouvelle app Ionic
ionic start passbi-app blank --type=angular
cd passbi-app
```

## Modèles de Données

```typescript
// models/passbi.models.ts
export interface Coordinates {
  lat: number;
  lon: number;
}

export interface RouteResult {
  duration_seconds: number;
  walk_distance_meters: number;
  transfers: number;
  steps: Step[];
}

export interface Step {
  type: 'WALK' | 'RIDE' | 'TRANSFER';
  from_stop: string;
  to_stop: string;
  from_stop_name: string;
  to_stop_name: string;
  route?: string;
  route_name?: string;
  mode?: string;
  duration_seconds: number;
  distance_meters?: number;
  num_stops?: number;
}

export interface RouteSearchResponse {
  routes: {
    no_transfer?: RouteResult;
    direct?: RouteResult;
    simple?: RouteResult;
    fast?: RouteResult;
  };
}

export interface NearbyStop {
  stop_id: string;
  name: string;
  lat: number;
  lon: number;
  distance_meters: number;
  routes: string[];
}

export interface NearbyStopsResponse {
  stops: NearbyStop[];
}

export enum RoutingStrategy {
  NO_TRANSFER = 'no_transfer',
  DIRECT = 'direct',
  SIMPLE = 'simple',
  FAST = 'fast'
}

export interface StrategyInfo {
  label: string;
  icon: string;
  color: string;
  description: string;
}
```

## Service PassBi

```typescript
// services/passbi.service.ts
import { Injectable } from '@angular/core';
import { HttpClient, HttpParams, HttpErrorResponse } from '@angular/common/http';
import { Observable, throwError } from 'rxjs';
import { catchError, retry, timeout } from 'rxjs/operators';
import {
  Coordinates,
  RouteSearchResponse,
  NearbyStopsResponse,
  RoutingStrategy,
  StrategyInfo
} from '../models/passbi.models';

@Injectable({
  providedIn: 'root'
})
export class PassBiService {
  private baseUrl = 'http://localhost:8080';
  private readonly timeout = 15000;

  // Informations sur les stratégies de routing
  readonly strategyInfo: Record<RoutingStrategy, StrategyInfo> = {
    [RoutingStrategy.NO_TRANSFER]: {
      label: 'Sans Transfert',
      icon: 'bed-outline',
      color: 'success',
      description: 'Ligne directe, maximum de confort'
    },
    [RoutingStrategy.DIRECT]: {
      label: 'Direct',
      icon: 'arrow-forward-outline',
      color: 'tertiary',
      description: 'Minimum de transferts'
    },
    [RoutingStrategy.SIMPLE]: {
      label: 'Recommandé',
      icon: 'checkmark-circle-outline',
      color: 'primary',
      description: 'Équilibre optimal temps/confort'
    },
    [RoutingStrategy.FAST]: {
      label: 'Plus Rapide',
      icon: 'flash-outline',
      color: 'warning',
      description: 'Temps de trajet minimum'
    }
  };

  constructor(private http: HttpClient) {}

  /**
   * Recherche d'itinéraires entre deux points
   */
  searchRoutes(from: Coordinates, to: Coordinates): Observable<RouteSearchResponse> {
    const params = new HttpParams()
      .set('from', `${from.lat},${from.lon}`)
      .set('to', `${to.lat},${to.lon}`);

    return this.http.get<RouteSearchResponse>(`${this.baseUrl}/v2/route-search`, { params })
      .pipe(
        timeout(this.timeout),
        retry(2),
        catchError(this.handleError)
      );
  }

  /**
   * Recherche d'arrêts à proximité
   */
  findNearbyStops(
    location: Coordinates,
    radius: number = 500
  ): Observable<NearbyStopsResponse> {
    const params = new HttpParams()
      .set('lat', location.lat.toString())
      .set('lon', location.lon.toString())
      .set('radius', radius.toString());

    return this.http.get<NearbyStopsResponse>(`${this.baseUrl}/v2/stops/nearby`, { params })
      .pipe(
        timeout(this.timeout),
        retry(2),
        catchError(this.handleError)
      );
  }

  /**
   * Vérification de la santé de l'API
   */
  checkHealth(): Observable<any> {
    return this.http.get(`${this.baseUrl}/health`)
      .pipe(
        timeout(5000),
        catchError(this.handleError)
      );
  }

  /**
   * Gestion des erreurs HTTP
   */
  private handleError(error: HttpErrorResponse) {
    let errorMessage = 'Une erreur est survenue';

    if (error.error instanceof ErrorEvent) {
      // Erreur côté client
      errorMessage = `Erreur: ${error.error.message}`;
    } else {
      // Erreur côté serveur
      switch (error.status) {
        case 400:
          errorMessage = 'Paramètres invalides';
          break;
        case 404:
          errorMessage = 'Aucun itinéraire trouvé';
          break;
        case 500:
          errorMessage = 'Erreur serveur';
          break;
        case 503:
          errorMessage = 'Service temporairement indisponible';
          break;
        default:
          errorMessage = `Erreur ${error.status}: ${error.message}`;
      }
    }

    return throwError(() => new Error(errorMessage));
  }

  /**
   * Formater la durée en minutes
   */
  formatDuration(seconds: number): string {
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) {
      return `${minutes} min`;
    }
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    return `${hours}h ${mins}min`;
  }

  /**
   * Formater la distance
   */
  formatDistance(meters: number): string {
    if (meters < 1000) {
      return `${Math.round(meters)} m`;
    }
    return `${(meters / 1000).toFixed(1)} km`;
  }
}
```

## Page de Recherche d'Itinéraire

```typescript
// pages/route-search/route-search.page.ts
import { Component, OnInit } from '@angular/core';
import { LoadingController, ToastController } from '@ionic/angular';
import { PassBiService } from '../../services/passbi.service';
import {
  Coordinates,
  RouteResult,
  RoutingStrategy
} from '../../models/passbi.models';

@Component({
  selector: 'app-route-search',
  templateUrl: './route-search.page.html',
  styleUrls: ['./route-search.page.scss'],
})
export class RouteSearchPage implements OnInit {
  fromLocation: Coordinates = { lat: 14.7167, lon: -17.4677 }; // Dakar
  toLocation: Coordinates = { lat: 14.6928, lon: -17.4467 }; // Almadies

  routes: Record<string, RouteResult> | null = null;
  selectedStrategy: RoutingStrategy = RoutingStrategy.SIMPLE;

  readonly RoutingStrategy = RoutingStrategy;
  readonly strategies = Object.values(RoutingStrategy);

  constructor(
    public passBiService: PassBiService,
    private loadingCtrl: LoadingController,
    private toastCtrl: ToastController
  ) {}

  ngOnInit() {
    this.searchRoutes();
  }

  async searchRoutes() {
    const loading = await this.loadingCtrl.create({
      message: 'Recherche d\'itinéraires...',
      spinner: 'crescent'
    });
    await loading.present();

    this.passBiService.searchRoutes(this.fromLocation, this.toLocation)
      .subscribe({
        next: (response) => {
          this.routes = response.routes;
          loading.dismiss();
        },
        error: async (error) => {
          loading.dismiss();
          const toast = await this.toastCtrl.create({
            message: error.message,
            duration: 3000,
            color: 'danger',
            position: 'top'
          });
          await toast.present();
        }
      });
  }

  getRouteForStrategy(strategy: RoutingStrategy): RouteResult | undefined {
    return this.routes?.[strategy];
  }

  selectStrategy(strategy: RoutingStrategy) {
    this.selectedStrategy = strategy;
  }

  getStepIcon(stepType: string): string {
    switch (stepType) {
      case 'WALK': return 'walk-outline';
      case 'RIDE': return 'bus-outline';
      case 'TRANSFER': return 'swap-horizontal-outline';
      default: return 'help-outline';
    }
  }

  getStepColor(stepType: string): string {
    switch (stepType) {
      case 'WALK': return 'primary';
      case 'RIDE': return 'success';
      case 'TRANSFER': return 'warning';
      default: return 'medium';
    }
  }
}
```

## Template de la Page

```html
<!-- pages/route-search/route-search.page.html -->
<ion-header>
  <ion-toolbar color="primary">
    <ion-title>🚌 PassBi - Planificateur</ion-title>
  </ion-toolbar>
</ion-header>

<ion-content [fullscreen]="true">
  <ion-refresher slot="fixed" (ionRefresh)="searchRoutes()">
    <ion-refresher-content></ion-refresher-content>
  </ion-refresher>

  <!-- Sélection des stratégies -->
  <ion-segment
    [(ngModel)]="selectedStrategy"
    mode="md"
    class="ion-padding"
  >
    <ion-segment-button
      *ngFor="let strategy of strategies"
      [value]="strategy"
    >
      <ion-label>
        <ion-icon [name]="passBiService.strategyInfo[strategy].icon"></ion-icon>
        <div>{{ passBiService.strategyInfo[strategy].label }}</div>
      </ion-label>
    </ion-segment-button>
  </ion-segment>

  <!-- Carte de l'itinéraire sélectionné -->
  <ion-card *ngIf="getRouteForStrategy(selectedStrategy) as route">
    <ion-card-header>
      <ion-card-subtitle>
        <ion-icon [name]="passBiService.strategyInfo[selectedStrategy].icon"></ion-icon>
        {{ passBiService.strategyInfo[selectedStrategy].label }}
      </ion-card-subtitle>
      <ion-card-title>
        {{ passBiService.formatDuration(route.duration_seconds) }}
      </ion-card-title>
    </ion-card-header>

    <ion-card-content>
      <!-- Résumé -->
      <ion-grid>
        <ion-row>
          <ion-col size="4">
            <div class="stat">
              <ion-icon name="time-outline"></ion-icon>
              <div class="stat-label">Durée</div>
              <div class="stat-value">
                {{ passBiService.formatDuration(route.duration_seconds) }}
              </div>
            </div>
          </ion-col>
          <ion-col size="4">
            <div class="stat">
              <ion-icon name="walk-outline"></ion-icon>
              <div class="stat-label">Marche</div>
              <div class="stat-value">
                {{ passBiService.formatDistance(route.walk_distance_meters) }}
              </div>
            </div>
          </ion-col>
          <ion-col size="4">
            <div class="stat">
              <ion-icon name="swap-horizontal-outline"></ion-icon>
              <div class="stat-label">Transferts</div>
              <div class="stat-value">{{ route.transfers }}</div>
            </div>
          </ion-col>
        </ion-row>
      </ion-grid>

      <!-- Étapes détaillées -->
      <ion-list class="steps-list">
        <ion-item *ngFor="let step of route.steps; let i = index" lines="none">
          <ion-icon
            slot="start"
            [name]="getStepIcon(step.type)"
            [color]="getStepColor(step.type)"
            size="large"
          ></ion-icon>

          <ion-label>
            <!-- Marche -->
            <h2 *ngIf="step.type === 'WALK'">
              Marcher {{ passBiService.formatDistance(step.distance_meters || 0) }}
            </h2>
            <p *ngIf="step.type === 'WALK'">
              De {{ step.from_stop_name }} à {{ step.to_stop_name }}
            </p>

            <!-- Transport -->
            <h2 *ngIf="step.type === 'RIDE'">
              Prendre {{ step.route_name }}
            </h2>
            <p *ngIf="step.type === 'RIDE'">
              De {{ step.from_stop_name }} à {{ step.to_stop_name }}
            </p>
            <p *ngIf="step.type === 'RIDE'" class="ride-details">
              <ion-badge color="medium">{{ step.num_stops }} arrêts</ion-badge>
              <ion-badge color="medium">{{ step.mode }}</ion-badge>
            </p>

            <!-- Transfert -->
            <h2 *ngIf="step.type === 'TRANSFER'">
              Transfert
            </h2>
            <p *ngIf="step.type === 'TRANSFER'">
              À {{ step.from_stop_name }}
            </p>

            <!-- Durée -->
            <p class="step-duration">
              <ion-icon name="time-outline" size="small"></ion-icon>
              {{ passBiService.formatDuration(step.duration_seconds) }}
            </p>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-card-content>
  </ion-card>

  <!-- Grille de toutes les options -->
  <div class="routes-grid ion-padding">
    <h2>Toutes les Options</h2>
    <ion-grid>
      <ion-row>
        <ion-col
          size="12"
          size-md="6"
          *ngFor="let strategy of strategies"
        >
          <ion-card
            button
            (click)="selectStrategy(strategy)"
            [class.selected]="selectedStrategy === strategy"
            *ngIf="getRouteForStrategy(strategy) as route"
          >
            <ion-card-header>
              <ion-card-subtitle>
                <ion-icon [name]="passBiService.strategyInfo[strategy].icon"></ion-icon>
                {{ passBiService.strategyInfo[strategy].label }}
              </ion-card-subtitle>
              <ion-card-title>
                {{ passBiService.formatDuration(route.duration_seconds) }}
              </ion-card-title>
            </ion-card-header>
            <ion-card-content>
              <ion-chip color="medium">
                <ion-icon name="walk-outline"></ion-icon>
                <ion-label>{{ passBiService.formatDistance(route.walk_distance_meters) }}</ion-label>
              </ion-chip>
              <ion-chip color="medium">
                <ion-icon name="swap-horizontal-outline"></ion-icon>
                <ion-label>{{ route.transfers }} transfert(s)</ion-label>
              </ion-chip>
            </ion-card-content>
          </ion-card>
        </ion-col>
      </ion-row>
    </ion-grid>
  </div>

  <!-- Message si aucun itinéraire -->
  <div class="no-routes ion-text-center ion-padding" *ngIf="!routes">
    <ion-icon name="search-outline" size="large"></ion-icon>
    <p>Aucun itinéraire trouvé</p>
  </div>
</ion-content>

<ion-footer>
  <ion-toolbar>
    <ion-button expand="block" (click)="searchRoutes()" fill="solid">
      <ion-icon slot="start" name="refresh-outline"></ion-icon>
      Actualiser
    </ion-button>
  </ion-toolbar>
</ion-footer>
```

## Styles de la Page

```scss
// pages/route-search/route-search.page.scss
.stat {
  text-align: center;
  padding: 1rem 0;

  ion-icon {
    font-size: 2rem;
    color: var(--ion-color-primary);
    margin-bottom: 0.5rem;
  }

  .stat-label {
    font-size: 0.75rem;
    color: var(--ion-color-medium);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 0.25rem;
  }

  .stat-value {
    font-size: 1.125rem;
    font-weight: 600;
    color: var(--ion-color-dark);
  }
}

.steps-list {
  margin-top: 1.5rem;
  background: transparent;

  ion-item {
    --background: transparent;
    margin-bottom: 1rem;
  }

  h2 {
    font-weight: 600;
    color: var(--ion-color-dark);
    margin-bottom: 0.25rem;
  }

  p {
    color: var(--ion-color-medium);
    font-size: 0.875rem;
  }

  .ride-details {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .step-duration {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    margin-top: 0.5rem;
    font-weight: 500;
    color: var(--ion-color-primary);
  }
}

.routes-grid {
  h2 {
    margin-bottom: 1rem;
    color: var(--ion-color-dark);
  }

  ion-card {
    transition: all 0.2s;

    &.selected {
      border: 2px solid var(--ion-color-primary);
      box-shadow: 0 4px 16px rgba(var(--ion-color-primary-rgb), 0.3);
    }

    &:hover {
      transform: translateY(-2px);
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
    }
  }
}

.no-routes {
  padding: 4rem 2rem;
  color: var(--ion-color-medium);

  ion-icon {
    font-size: 4rem;
    margin-bottom: 1rem;
  }
}

ion-segment {
  ion-segment-button {
    ion-label {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 0.25rem;

      ion-icon {
        font-size: 1.5rem;
      }

      div {
        font-size: 0.75rem;
      }
    }
  }
}
```

## Module de la Page

```typescript
// pages/route-search/route-search.module.ts
import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { IonicModule } from '@ionic/angular';
import { RouteSearchPageRoutingModule } from './route-search-routing.module';
import { RouteSearchPage } from './route-search.page';

@NgModule({
  imports: [
    CommonModule,
    FormsModule,
    IonicModule,
    RouteSearchPageRoutingModule
  ],
  declarations: [RouteSearchPage]
})
export class RouteSearchPageModule {}
```

## Configuration du Module Principal

```typescript
// app.module.ts
import { NgModule } from '@angular/core';
import { BrowserModule } from '@angular/platform-browser';
import { RouteReuseStrategy } from '@angular/router';
import { HttpClientModule } from '@angular/common/http';

import { IonicModule, IonicRouteStrategy } from '@ionic/angular';

import { AppComponent } from './app.component';
import { AppRoutingModule } from './app-routing.module';
import { PassBiService } from './services/passbi.service';

@NgModule({
  declarations: [AppComponent],
  imports: [
    BrowserModule,
    IonicModule.forRoot(),
    AppRoutingModule,
    HttpClientModule
  ],
  providers: [
    { provide: RouteReuseStrategy, useClass: IonicRouteStrategy },
    PassBiService
  ],
  bootstrap: [AppComponent],
})
export class AppModule {}
```

## Composant d'Arrêts à Proximité

```typescript
// components/nearby-stops/nearby-stops.component.ts
import { Component, Input, OnInit } from '@angular/core';
import { PassBiService } from '../../services/passbi.service';
import { Coordinates, NearbyStop } from '../../models/passbi.models';

@Component({
  selector: 'app-nearby-stops',
  templateUrl: './nearby-stops.component.html',
  styleUrls: ['./nearby-stops.component.scss'],
})
export class NearbyStopsComponent implements OnInit {
  @Input() location!: Coordinates;
  @Input() radius: number = 500;

  stops: NearbyStop[] = [];
  loading = false;
  error: string | null = null;

  constructor(public passBiService: PassBiService) {}

  ngOnInit() {
    this.loadNearbyStops();
  }

  loadNearbyStops() {
    this.loading = true;
    this.error = null;

    this.passBiService.findNearbyStops(this.location, this.radius)
      .subscribe({
        next: (response) => {
          this.stops = response.stops;
          this.loading = false;
        },
        error: (error) => {
          this.error = error.message;
          this.loading = false;
        }
      });
  }
}
```

```html
<!-- components/nearby-stops/nearby-stops.component.html -->
<ion-list>
  <ion-list-header>
    <ion-label>Arrêts à Proximité</ion-label>
  </ion-list-header>

  <ion-item *ngIf="loading">
    <ion-spinner></ion-spinner>
    <ion-label class="ion-padding-start">Recherche...</ion-label>
  </ion-item>

  <ion-item *ngIf="error" color="danger">
    <ion-icon name="alert-circle-outline" slot="start"></ion-icon>
    <ion-label>{{ error }}</ion-label>
  </ion-item>

  <ion-item *ngFor="let stop of stops" button>
    <ion-icon name="location-outline" slot="start" color="primary"></ion-icon>
    <ion-label>
      <h2>{{ stop.name }}</h2>
      <p>{{ passBiService.formatDistance(stop.distance_meters) }}</p>
      <div class="routes-chips">
        <ion-chip *ngFor="let route of stop.routes" color="primary" outline>
          <ion-label>{{ route }}</ion-label>
        </ion-chip>
      </div>
    </ion-label>
  </ion-item>

  <ion-item *ngIf="stops.length === 0 && !loading && !error">
    <ion-label class="ion-text-center">
      <p>Aucun arrêt trouvé dans un rayon de {{ radius }}m</p>
    </ion-label>
  </ion-item>
</ion-list>
```

## Utilisation de Capacitor pour la Géolocalisation

```typescript
// services/geolocation.service.ts
import { Injectable } from '@angular/core';
import { Geolocation } from '@capacitor/geolocation';
import { Coordinates } from '../models/passbi.models';

@Injectable({
  providedIn: 'root'
})
export class GeolocationService {

  async getCurrentPosition(): Promise<Coordinates> {
    try {
      const position = await Geolocation.getCurrentPosition();
      return {
        lat: position.coords.latitude,
        lon: position.coords.longitude
      };
    } catch (error) {
      throw new Error('Impossible d\'obtenir la position actuelle');
    }
  }

  async checkPermissions(): Promise<boolean> {
    const permissions = await Geolocation.checkPermissions();
    return permissions.location === 'granted';
  }

  async requestPermissions(): Promise<boolean> {
    const permissions = await Geolocation.requestPermissions();
    return permissions.location === 'granted';
  }
}
```

## Exemple d'Application Complète

```typescript
// app.component.ts
import { Component, OnInit } from '@angular/core';
import { PassBiService } from './services/passbi.service';
import { ToastController } from '@ionic/angular';

@Component({
  selector: 'app-root',
  templateUrl: 'app.component.html',
  styleUrls: ['app.component.scss'],
})
export class AppComponent implements OnInit {
  apiHealthy = false;

  constructor(
    private passBiService: PassBiService,
    private toastCtrl: ToastController
  ) {}

  ngOnInit() {
    this.checkApiHealth();
  }

  checkApiHealth() {
    this.passBiService.checkHealth().subscribe({
      next: () => {
        this.apiHealthy = true;
      },
      error: async () => {
        const toast = await this.toastCtrl.create({
          message: 'API PassBi indisponible',
          duration: 3000,
          color: 'warning',
          position: 'top'
        });
        await toast.present();
      }
    });
  }
}
```

## Configuration pour Production

```typescript
// environments/environment.prod.ts
export const environment = {
  production: true,
  passBiApiUrl: 'https://api.passbi.com'
};

// environments/environment.ts
export const environment = {
  production: false,
  passBiApiUrl: 'http://localhost:8080'
};

// Mise à jour du service
import { environment } from '../../environments/environment';

@Injectable({
  providedIn: 'root'
})
export class PassBiService {
  private baseUrl = environment.passBiApiUrl;
  // ...
}
```

## Voir Aussi

- [React Examples](react.md) - Exemples React
- [Flutter Examples](flutter.md) - Exemples Flutter
- [Integration Guide](../../guides/integration-guide.md) - Guide complet
- [Error Reference](../reference/errors.md) - Gestion des erreurs
