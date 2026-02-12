# Guide d'Onboarding Partenaire PassBI

Bienvenue sur PassBI ! Ce guide vous aidera à intégrer notre API de routage multimodal dans votre application.

---

## 🚀 Démarrage Rapide (5 minutes)

### Étape 1 : Créer votre Compte Partenaire

1. Rendez-vous sur [https://partners.passbi.com/signup](https://partners.passbi.com/signup)
2. Remplissez le formulaire d'inscription
3. Vérifiez votre email
4. Choisissez votre plan (Free pour commencer)

### Étape 2 : Obtenir votre API Key

1. Connectez-vous à votre [Dashboard Partenaire](https://partners.passbi.com/dashboard)
2. Cliquez sur "API Keys" dans le menu
3. Cliquez sur "Créer une nouvelle clé"
4. Donnez-lui un nom (ex: "Production")
5. **Copiez immédiatement la clé** (vous ne pourrez plus la voir après !)

Format de la clé : `pk_live_abc123...`

⚠️ **Important** : Ne partagez jamais votre clé API. Gardez-la secrète comme un mot de passe.

### Étape 3 : Faire votre Premier Appel API

#### Option A : Avec cURL

```bash
curl -X GET "https://api.passbi.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" \
  -H "Authorization: Bearer pk_live_VOTRE_CLE"
```

#### Option B : Avec JavaScript

```javascript
const PassBiClient = require('./passbi-client');

const client = new PassBiClient('pk_live_VOTRE_CLE');

const routes = await client.searchRoutes({
    from: '14.7167,-17.4677',
    to: '14.6928,-17.4467'
});

console.log(routes);
```

#### Option C : Avec Python

```python
from passbi_client import PassBiClient

client = PassBiClient('pk_live_VOTRE_CLE')

routes = client.search_routes(
    from_coords='14.7167,-17.4677',
    to_coords='14.6928,-17.4467'
)

print(routes)
```

---

## 📚 Fonctionnalités Principales

### 1. Recherche de Trajets

Trouvez le meilleur itinéraire entre deux points avec 4 stratégies différentes :

- **no_transfer** : Sans correspondance (trajet direct uniquement)
- **direct** : Minimise les correspondances
- **simple** : Équilibre temps et correspondances
- **fast** : Minimise le temps total

```javascript
const routes = await client.searchRoutes({
    from: '14.7167,-17.4677',  // Origine (lat,lon)
    to: '14.6928,-17.4467'     // Destination (lat,lon)
});

// Afficher les différentes options
console.log('Sans correspondance:', routes.routes.no_transfer);
console.log('Direct:', routes.routes.direct);
console.log('Simple:', routes.routes.simple);
console.log('Rapide:', routes.routes.fast);
```

### 2. Arrêts à Proximité

Trouvez les arrêts de transport autour d'un point :

```javascript
const stops = await client.findNearbyStops({
    lat: 14.6928,
    lon: -17.4467,
    radius: 500  // Rayon en mètres (max: 5000)
});

console.log(`${stops.stops.length} arrêts trouvés`);
stops.stops.forEach(stop => {
    console.log(`- ${stop.name} (${stop.distance_meters}m)`);
});
```

### 3. Liste des Lignes

Obtenez la liste de toutes les lignes disponibles :

```javascript
const routes = await client.listRoutes({
    mode: 'BUS',    // Optionnel : BUS, BRT, TER
    limit: 20       // Nombre de résultats
});

routes.routes.forEach(route => {
    console.log(`${route.name} - ${route.mode}`);
});
```

---

## 💡 Exemples d'Utilisation

### Cas 1 : Application Mobile de Transport

```javascript
// 1. Obtenir la position de l'utilisateur
const userPosition = await getUserLocation();

// 2. Trouver les arrêts à proximité
const nearbyStops = await client.findNearbyStops({
    lat: userPosition.lat,
    lon: userPosition.lon,
    radius: 300
});

// 3. Afficher les arrêts sur la carte
displayStopsOnMap(nearbyStops.stops);

// 4. Calculer un itinéraire
const destination = selectDestination();
const routes = await client.searchRoutes({
    from: `${userPosition.lat},${userPosition.lon}`,
    to: `${destination.lat},${destination.lon}`
});

// 5. Afficher les options à l'utilisateur
displayRouteOptions(routes.routes);
```

### Cas 2 : Site Web de Planification

```javascript
// Fonction pour calculer plusieurs trajets
async function planMultipleTrips(origins, destinations) {
    const results = [];

    for (const origin of origins) {
        for (const destination of destinations) {
            const routes = await client.searchRoutes({
                from: origin,
                to: destination
            });

            results.push({
                from: origin,
                to: destination,
                bestRoute: routes.routes.fast,
                alternatives: routes.routes
            });

            // Respecter le rate limit
            await sleep(100);
        }
    }

    return results;
}
```

### Cas 3 : Backend de Recommandation

```python
from passbi_client import PassBiClient

client = PassBiClient('pk_live_...')

def find_best_commute(home, workplaces):
    """Trouve le meilleur trajet domicile-travail"""
    results = []

    for workplace in workplaces:
        routes = client.search_routes(
            from_coords=home,
            to_coords=workplace['coords']
        )

        # Calculer un score basé sur le temps et les correspondances
        best = routes['routes']['simple']
        score = calculate_score(
            duration=best['duration_seconds'],
            transfers=best['transfers'],
            walk_distance=best['walk_distance_meters']
        )

        results.append({
            'workplace': workplace['name'],
            'score': score,
            'duration_minutes': best['duration_seconds'] // 60,
            'transfers': best['transfers']
        })

    return sorted(results, key=lambda x: x['score'], reverse=True)

def calculate_score(duration, transfers, walk_distance):
    """Calcule un score de qualité du trajet"""
    # Pénalités
    time_penalty = duration / 60  # 1 point par minute
    transfer_penalty = transfers * 5  # 5 points par correspondance
    walk_penalty = walk_distance / 100  # 1 point par 100m

    # Score (plus c'est bas, mieux c'est)
    return time_penalty + transfer_penalty + walk_penalty
```

---

## 📊 Gestion de votre Compte

### Consulter vos Statistiques

```javascript
// Obtenir les stats des 30 derniers jours
const stats = await client.getUsageStats({ days: 30 });

console.log('Statistiques:');
stats.stats.forEach(day => {
    console.log(`${day.date}: ${day.total_requests} requêtes`);
    console.log(`  - Succès: ${day.successful}`);
    console.log(`  - Temps moyen: ${day.avg_response_time_ms}ms`);
    console.log(`  - Cache hit: ${day.cache_hit_rate}%`);
});
```

### Vérifier vos Quotas

```javascript
const quota = await client.getQuotaUsage();

console.log('Quota Journalier:');
console.log(`  Utilisé: ${quota.daily.requests}/${quota.daily.limit}`);
console.log(`  Restant: ${quota.daily.remaining}`);

console.log('Quota Mensuel:');
console.log(`  Utilisé: ${quota.monthly.requests}/${quota.monthly.limit}`);
console.log(`  Restant: ${quota.monthly.remaining}`);
```

### Gérer vos API Keys

```javascript
// Créer une nouvelle clé
const newKey = await client.createAPIKey({
    name: 'Mobile App Production',
    description: 'Clé pour l\'app mobile iOS/Android',
    scopes: ['read:routes'],
    expiresAt: new Date('2026-12-31')
});

console.log('⚠️ Sauvegardez cette clé:', newKey.api_key);

// Lister toutes vos clés
const keys = await client.listAPIKeys();
keys.api_keys.forEach(key => {
    console.log(`${key.name}: ${key.key_prefix}`);
    console.log(`  Active: ${key.is_active}`);
    console.log(`  Dernière utilisation: ${key.last_used_at}`);
});

// Révoquer une clé
await client.revokeAPIKey('key_id_123');
```

---

## ⚡ Rate Limits et Quotas

### Limites par Plan

| Plan | Requêtes/sec | Requêtes/jour | Requêtes/mois |
|------|--------------|---------------|---------------|
| Free | 2 | 1,000 | 30,000 |
| Starter | 10 | 10,000 | 300,000 |
| Business | 50 | 50,000 | 1,500,000 |
| Enterprise | 1,000 | Illimité | Illimité |

### Gérer les Rate Limits

```javascript
try {
    const routes = await client.searchRoutes({ from, to });

    // Vérifier les limites
    const rateInfo = client.getRateLimitInfo();
    console.log(`Restant aujourd'hui: ${rateInfo.remainingDay}`);

    // Avertir si proche de la limite
    if (rateInfo.remainingDay < 100) {
        console.warn('⚠️ Attention: Proche de la limite journalière');
    }

} catch (error) {
    if (error.isRateLimitError()) {
        console.error('Rate limit dépassé!');
        console.error('Réessayez dans:', error.details.retry_after, 'secondes');

        // Attendre et réessayer
        await sleep(error.details.retry_after * 1000);
        return await client.searchRoutes({ from, to });
    }
}
```

### Best Practices

1. **Mise en cache** : Cachez les résultats côté client pour éviter les requêtes répétées
2. **Batch processing** : Groupez vos requêtes si possible
3. **Rate limit monitoring** : Surveillez vos headers de rate limit
4. **Retry logic** : Implémentez une logique de retry avec backoff exponentiel

```javascript
async function searchWithRetry(client, params, maxRetries = 3) {
    for (let i = 0; i < maxRetries; i++) {
        try {
            return await client.searchRoutes(params);
        } catch (error) {
            if (error.isRateLimitError() && i < maxRetries - 1) {
                const delay = Math.pow(2, i) * 1000; // Backoff exponentiel
                await sleep(delay);
                continue;
            }
            throw error;
        }
    }
}
```

---

## 🔒 Sécurité

### Bonnes Pratiques

✅ **À FAIRE**
- Stocker votre API key dans les variables d'environnement
- Utiliser HTTPS pour toutes les requêtes
- Révoquer immédiatement les clés compromises
- Créer des clés différentes pour dev/staging/production
- Monitorer l'utilisation de vos clés

❌ **À NE PAS FAIRE**
- Exposer votre API key dans le code frontend
- Committer vos clés dans Git
- Partager vos clés par email/chat
- Utiliser la même clé partout
- Ignorer les alertes de sécurité

### Stockage Sécurisé

**Node.js (Backend)**
```javascript
// .env
PASSBI_API_KEY=pk_live_abc123...

// app.js
require('dotenv').config();
const client = new PassBiClient(process.env.PASSBI_API_KEY);
```

**Python**
```python
# .env
PASSBI_API_KEY=pk_live_abc123...

# app.py
import os
from dotenv import load_dotenv

load_dotenv()
client = PassBiClient(os.getenv('PASSBI_API_KEY'))
```

**Frontend (Proxy via Backend)**
```javascript
// ❌ MAUVAIS - Ne pas faire ça
const client = new PassBiClient('pk_live_abc123...'); // Exposé dans le code!

// ✅ BON - Créer un proxy backend
// Backend Express
app.get('/api/routes', async (req, res) => {
    const routes = await passBiClient.searchRoutes(req.query);
    res.json(routes);
});

// Frontend
const routes = await fetch('/api/routes?from=...&to=...');
```

---

## 🐛 Gestion des Erreurs

### Types d'Erreurs

| Code | Erreur | Description | Action |
|------|--------|-------------|--------|
| 401 | `invalid_api_key` | Clé API invalide | Vérifier votre clé |
| 403 | `insufficient_permissions` | Permissions insuffisantes | Vérifier les scopes |
| 429 | `rate_limit_exceeded` | Rate limit dépassé | Attendre et réessayer |
| 429 | `daily_quota_exceeded` | Quota journalier dépassé | Upgrader votre plan |
| 404 | `no_routes_found` | Aucun trajet trouvé | Vérifier les coordonnées |
| 500 | `internal_server_error` | Erreur serveur | Contacter le support |

### Gestion Complète des Erreurs

```javascript
async function safeSearchRoutes(from, to) {
    try {
        const routes = await client.searchRoutes({ from, to });
        return { success: true, data: routes };

    } catch (error) {
        // Log l'erreur
        console.error('Erreur PassBi:', error);

        // Gérer selon le type
        if (error.isRateLimitError()) {
            return {
                success: false,
                error: 'rate_limit',
                message: 'Trop de requêtes, réessayez dans quelques instants',
                retryAfter: error.details.retry_after
            };
        }

        if (error.isAuthError()) {
            return {
                success: false,
                error: 'auth',
                message: 'Problème d\'authentification'
            };
        }

        if (error.statusCode === 404) {
            return {
                success: false,
                error: 'not_found',
                message: 'Aucun trajet trouvé pour ces coordonnées'
            };
        }

        // Erreur générique
        return {
            success: false,
            error: 'unknown',
            message: 'Une erreur est survenue'
        };
    }
}
```

---

## 📈 Upgrade de Plan

### Quand Upgrader ?

Considérez un upgrade si :
- ⚠️ Vous atteignez 80% de votre quota mensuel
- ⚠️ Vous êtes fréquemment rate-limité
- ⚠️ Vous avez besoin de support prioritaire
- ⚠️ Vous avez besoin de webhooks

### Comparaison des Plans

| Feature | Free | Starter | Business | Enterprise |
|---------|------|---------|----------|------------|
| Requêtes/mois | 30K | 300K | 1.5M | Illimité |
| Rate limit/sec | 2 | 10 | 50 | 1000 |
| API Keys | 2 | 5 | 20 | Illimité |
| Support | Community | Email | Email + Chat | Dedicated |
| SLA | - | 99% | 99.5% | 99.9% |
| Webhooks | ❌ | ✅ | ✅ | ✅ |
| Prix/mois | 0€ | 49€ | 199€ | Sur mesure |

[Upgrader mon plan →](https://partners.passbi.com/billing)

---

## 🆘 Support

### Documentation
- [📖 Documentation Complète](https://docs.passbi.com)
- [🔧 Référence API](https://docs.passbi.com/api)
- [💡 Exemples de Code](https://docs.passbi.com/examples)

### Communauté
- [💬 Forum Communautaire](https://community.passbi.com)
- [📺 Tutoriels Vidéo](https://youtube.com/@passbi)
- [💻 GitHub](https://github.com/passbi)

### Contact
- **Email** : [partners@passbi.com](mailto:partners@passbi.com)
- **Chat** : [Starter plan et +]
- **Phone** : [Enterprise plan uniquement]

### Status
- [🔴🟢 Status Page](https://status.passbi.com)
- [📊 Incidents](https://status.passbi.com/incidents)

---

## ✅ Checklist de Lancement

Avant de passer en production :

- [ ] API key production créée et sécurisée
- [ ] Variables d'environnement configurées
- [ ] Gestion des erreurs implémentée
- [ ] Rate limiting et retry logic en place
- [ ] Monitoring des quotas configuré
- [ ] Cache implémenté côté client
- [ ] Tests de charge effectués
- [ ] Plan adapté à votre trafic
- [ ] Équipe formée sur l'API
- [ ] Support contacté pour validation

---

**Bienvenue dans la famille PassBI ! 🎉**

Pour toute question, n'hésitez pas à nous contacter : [partners@passbi.com](mailto:partners@passbi.com)
