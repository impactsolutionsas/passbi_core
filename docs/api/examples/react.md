# Exemples React

Intégration complète de PassBi API dans une application React.

## Installation

```bash
npm install axios
# ou
npm install react-query axios
```

## Hook Personnalisé `usePassBi`

```typescript
// hooks/usePassBi.ts
import { useState, useEffect } from 'react';

interface Coordinates {
  lat: number;
  lon: number;
}

interface RouteResult {
  duration_seconds: number;
  walk_distance_meters: number;
  transfers: number;
  steps: Step[];
}

interface Step {
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

interface UseRouteSearchResult {
  routes: Record<string, RouteResult> | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useRouteSearch(
  from: Coordinates | null,
  to: Coordinates | null
): UseRouteSearchResult {
  const [routes, setRoutes] = useState<Record<string, RouteResult> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refetchTrigger, setRefetchTrigger] = useState(0);

  const refetch = () => setRefetchTrigger(prev => prev + 1);

  useEffect(() => {
    if (!from || !to) {
      return;
    }

    let cancelled = false;

    const fetchRoutes = async () => {
      setLoading(true);
      setError(null);

      try {
        const url = new URL('http://localhost:8080/v2/route-search');
        url.searchParams.set('from', `${from.lat},${from.lon}`);
        url.searchParams.set('to', `${to.lat},${to.lon}`);

        const response = await fetch(url);

        if (!response.ok) {
          if (response.status === 404) {
            throw new Error('Aucun itinéraire trouvé entre ces locations');
          }
          const errorData = await response.json();
          throw new Error(errorData.error || `Erreur HTTP ${response.status}`);
        }

        const data = await response.json();

        if (!cancelled) {
          setRoutes(data.routes);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Erreur inconnue');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchRoutes();

    return () => {
      cancelled = true;
    };
  }, [from?.lat, from?.lon, to?.lat, to?.lon, refetchTrigger]);

  return { routes, loading, error, refetch };
}
```

## Composant de Recherche d'Itinéraire

```tsx
// components/RouteSearch.tsx
import React, { useState } from 'react';
import { useRouteSearch } from '../hooks/usePassBi';

export function RouteSearch() {
  const [from, setFrom] = useState({ lat: 14.7167, lon: -17.4677 });
  const [to, setTo] = useState({ lat: 14.6928, lon: -17.4467 });

  const { routes, loading, error, refetch } = useRouteSearch(from, to);

  if (loading) {
    return (
      <div className="flex items-center justify-center p-8">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
        <p className="ml-4 text-gray-600">Recherche d'itinéraires...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <h3 className="text-red-800 font-semibold">Erreur</h3>
        <p className="text-red-600">{error}</p>
        <button
          onClick={refetch}
          className="mt-2 px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
        >
          Réessayer
        </button>
      </div>
    );
  }

  if (!routes) return null;

  return (
    <div className="space-y-6">
      <h2 className="text-2xl font-bold text-gray-900">Itinéraires Disponibles</h2>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {Object.entries(routes).map(([strategy, route]) => (
          <RouteCard key={strategy} strategy={strategy} route={route} />
        ))}
      </div>
    </div>
  );
}

interface RouteCardProps {
  strategy: string;
  route: RouteResult;
}

function RouteCard({ strategy, route }: RouteCardProps) {
  const strategyLabels: Record<string, { label: string; icon: string; color: string }> = {
    simple: { label: 'Recommandé', icon: '✓', color: 'blue' },
    fast: { label: 'Plus Rapide', icon: '⚡', color: 'yellow' },
    no_transfer: { label: 'Sans Transfert', icon: '🛋️', color: 'green' },
    direct: { label: 'Direct', icon: '➡️', color: 'gray' }
  };

  const info = strategyLabels[strategy];
  const durationMin = Math.floor(route.duration_seconds / 60);

  return (
    <div className="bg-white rounded-lg shadow-lg p-6 hover:shadow-xl transition-shadow">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <span className="text-2xl">{info.icon}</span>
          <h3 className="text-lg font-semibold text-gray-900">{info.label}</h3>
        </div>
        <span className={`px-3 py-1 rounded-full text-sm font-medium bg-${info.color}-100 text-${info.color}-800`}>
          {durationMin} min
        </span>
      </div>

      <div className="space-y-2 text-sm text-gray-600">
        <div className="flex justify-between">
          <span>Durée:</span>
          <span className="font-medium">{durationMin} minutes</span>
        </div>
        <div className="flex justify-between">
          <span>Marche:</span>
          <span className="font-medium">{route.walk_distance_meters}m</span>
        </div>
        <div className="flex justify-between">
          <span>Transferts:</span>
          <span className="font-medium">{route.transfers}</span>
        </div>
      </div>

      <div className="mt-4 pt-4 border-t border-gray-200">
        <button className="w-full px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors">
          Voir les détails
        </button>
      </div>
    </div>
  );
}
```

## Composant avec Détails des Étapes

```tsx
// components/RouteDetails.tsx
import React from 'react';

interface Step {
  type: 'WALK' | 'RIDE' | 'TRANSFER';
  from_stop_name: string;
  to_stop_name: string;
  route_name?: string;
  mode?: string;
  duration_seconds: number;
  distance_meters?: number;
  num_stops?: number;
}

interface RouteDetailsProps {
  steps: Step[];
}

export function RouteDetails({ steps }: RouteDetailsProps) {
  return (
    <div className="bg-white rounded-lg shadow p-6">
      <h3 className="text-xl font-bold mb-4">Itinéraire Détaillé</h3>

      <div className="space-y-4">
        {steps.map((step, index) => (
          <StepItem key={index} step={step} isLast={index === steps.length - 1} />
        ))}
      </div>
    </div>
  );
}

interface StepItemProps {
  step: Step;
  isLast: boolean;
}

function StepItem({ step, isLast }: StepItemProps) {
  const durationMin = Math.floor(step.duration_seconds / 60);

  const icons = {
    WALK: '🚶',
    RIDE: '🚌',
    TRANSFER: '🔄'
  };

  const colors = {
    WALK: 'blue',
    RIDE: 'green',
    TRANSFER: 'yellow'
  };

  return (
    <div className="flex gap-4">
      <div className="flex flex-col items-center">
        <div className={`w-10 h-10 rounded-full bg-${colors[step.type]}-100 flex items-center justify-center`}>
          <span className="text-xl">{icons[step.type]}</span>
        </div>
        {!isLast && (
          <div className="w-0.5 h-full bg-gray-300 my-1"></div>
        )}
      </div>

      <div className="flex-1 pb-4">
        {step.type === 'WALK' && (
          <div>
            <p className="font-medium text-gray-900">
              Marcher {step.distance_meters}m
            </p>
            <p className="text-sm text-gray-600">
              De {step.from_stop_name} à {step.to_stop_name}
            </p>
            <p className="text-xs text-gray-500">{durationMin} min</p>
          </div>
        )}

        {step.type === 'RIDE' && (
          <div>
            <p className="font-medium text-gray-900">
              Prendre {step.route_name}
            </p>
            <p className="text-sm text-gray-600">
              De {step.from_stop_name} à {step.to_stop_name}
            </p>
            <p className="text-xs text-gray-500">
              {step.num_stops} arrêts • {durationMin} min • {step.mode}
            </p>
          </div>
        )}

        {step.type === 'TRANSFER' && (
          <div>
            <p className="font-medium text-gray-900">
              Transfert
            </p>
            <p className="text-sm text-gray-600">
              À {step.from_stop_name}
            </p>
            <p className="text-xs text-gray-500">{durationMin} min d'attente</p>
          </div>
        )}
      </div>
    </div>
  );
}
```

## Context Provider pour PassBi

```tsx
// context/PassBiContext.tsx
import React, { createContext, useContext, ReactNode } from 'react';

interface PassBiConfig {
  baseURL: string;
  timeout?: number;
}

const PassBiContext = createContext<PassBiConfig>({
  baseURL: 'http://localhost:8080',
  timeout: 15000
});

export function PassBiProvider({
  children,
  config
}: {
  children: ReactNode;
  config?: Partial<PassBiConfig>;
}) {
  const value: PassBiConfig = {
    baseURL: config?.baseURL || 'http://localhost:8080',
    timeout: config?.timeout || 15000
  };

  return (
    <PassBiContext.Provider value={value}>
      {children}
    </PassBiContext.Provider>
  );
}

export function usePassBiConfig() {
  return useContext(PassBiContext);
}
```

## Utilisation avec React Query

```tsx
// hooks/usePassBiQuery.ts
import { useQuery } from 'react-query';
import { usePassBiConfig } from '../context/PassBiContext';

interface Coordinates {
  lat: number;
  lon: number;
}

export function useRouteSearchQuery(from: Coordinates | null, to: Coordinates | null) {
  const { baseURL } = usePassBiConfig();

  return useQuery(
    ['routes', from, to],
    async () => {
      if (!from || !to) return null;

      const url = new URL(`${baseURL}/v2/route-search`);
      url.searchParams.set('from', `${from.lat},${from.lon}`);
      url.searchParams.set('to', `${to.lat},${to.lon}`);

      const response = await fetch(url);

      if (!response.ok) {
        throw new Error('Failed to fetch routes');
      }

      return response.json();
    },
    {
      enabled: !!from && !!to,
      staleTime: 10 * 60 * 1000, // 10 minutes
      retry: 2
    }
  );
}
```

## Application Complète

```tsx
// App.tsx
import React from 'react';
import { PassBiProvider } from './context/PassBiContext';
import { RouteSearch } from './components/RouteSearch';

function App() {
  return (
    <PassBiProvider config={{ baseURL: 'http://localhost:8080' }}>
      <div className="min-h-screen bg-gray-50">
        <header className="bg-white shadow">
          <div className="max-w-7xl mx-auto py-6 px-4">
            <h1 className="text-3xl font-bold text-gray-900">
              🚌 PassBi - Planificateur de Trajet
            </h1>
          </div>
        </header>

        <main className="max-w-7xl mx-auto py-6 px-4">
          <RouteSearch />
        </main>
      </div>
    </PassBiProvider>
  );
}

export default App;
```

## Avec Tailwind CSS

Ajoutez Tailwind pour un style moderne :

```bash
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

```javascript
// tailwind.config.js
module.exports = {
  content: [
    "./src/**/*.{js,jsx,ts,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
```

## Gestion d'État avec Zustand

```typescript
// store/routeStore.ts
import create from 'zustand';

interface RouteStore {
  from: { lat: number; lon: number } | null;
  to: { lat: number; lon: number } | null;
  selectedStrategy: string;
  setFrom: (coords: { lat: number; lon: number }) => void;
  setTo: (coords: { lat: number; lon: number }) => void;
  setSelectedStrategy: (strategy: string) => void;
}

export const useRouteStore = create<RouteStore>((set) => ({
  from: null,
  to: null,
  selectedStrategy: 'simple',
  setFrom: (coords) => set({ from: coords }),
  setTo: (coords) => set({ to: coords }),
  setSelectedStrategy: (strategy) => set({ selectedStrategy: strategy })
}));
```

## Voir Aussi

- [JavaScript Examples](javascript.md) - Exemples JavaScript purs
- [Integration Guide](../../guides/integration-guide.md) - Guide complet
- [Error Reference](../reference/errors.md) - Gestion des erreurs
