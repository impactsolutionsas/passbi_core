# Guide d'Implémentation - Système API-as-a-Service

Ce guide vous accompagne dans le déploiement du système de gestion des partenaires pour PassBI.

---

## 📦 Fichiers Créés

### 1. **Migrations SQL**
- `migrations/002_partner_system.up.sql` - Création des tables
- `migrations/002_partner_system.down.sql` - Rollback

### 2. **Middlewares**
- `internal/middleware/auth.go` - Authentification API Key
- `internal/middleware/ratelimit.go` - Rate limiting multi-niveaux
- `internal/middleware/analytics.go` - Logging et analytics

### 3. **Handlers API**
- `internal/api/partner_dashboard.go` - Endpoints dashboard partenaire

### 4. **Serveur Principal**
- `cmd/api/main_with_auth.go` - Version avec authentification

### 5. **SDKs Clients**
- `sdks/javascript/passbi-client.js` - Client JavaScript/TypeScript
- `sdks/python/passbi_client.py` - Client Python

### 6. **Documentation**
- `docs/architecture/partner-api-architecture.md` - Architecture complète
- `docs/guides/partner-onboarding.md` - Guide partenaires

---

## 🚀 Déploiement Étape par Étape

### Phase 1 : Préparation Base de Données

#### 1.1 Exécuter les Migrations

```bash
# Se connecter à la base de données
export DATABASE_URL="postgresql://user:password@host:5432/passbi?sslmode=require"

# Exécuter la migration
migrate -path migrations -database $DATABASE_URL up

# Vérifier les tables créées
psql $DATABASE_URL -c "\dt partner*"
```

#### 1.2 Créer un Premier Partenaire de Test

```sql
-- Créer un partenaire de test
INSERT INTO partner (
    name, email, company, tier,
    rate_limit_per_second, rate_limit_per_day, rate_limit_per_month
) VALUES (
    'Test Partner', 'test@example.com', 'Test Company', 'free',
    2, 1000, 30000
) RETURNING id;

-- Créer une API key de test (remplacer PARTNER_ID)
INSERT INTO api_key (
    partner_id,
    key_hash,
    key_prefix,
    name,
    scopes
) VALUES (
    'PARTNER_ID',
    'HASH_DE_TEST', -- Générer avec SHA-256
    'pk_test_abc...',
    'Test Key',
    ARRAY['read:routes']
);
```

**Générer une vraie API key :**

```bash
# Utiliser le script Go
go run scripts/generate_api_key.go
```

Ou créer `scripts/generate_api_key.go` :

```go
package main

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
)

func main() {
    // Générer 32 bytes aléatoires
    randomBytes := make([]byte, 32)
    rand.Read(randomBytes)
    randomStr := hex.EncodeToString(randomBytes)

    // Générer checksum
    checksumBytes := sha256.Sum256([]byte(randomStr))
    checksum := hex.EncodeToString(checksumBytes[:2])

    // Construire la clé
    key := fmt.Sprintf("pk_test_%s_%s", randomStr, checksum)

    // Hasher pour stockage
    hashBytes := sha256.Sum256([]byte(key))
    hash := hex.EncodeToString(hashBytes[:])

    fmt.Println("API Key:", key)
    fmt.Println("Hash:", hash)
    fmt.Println("Prefix:", fmt.Sprintf("pk_test_%s...", randomStr[:8]))
}
```

---

### Phase 2 : Configuration de l'Application

#### 2.1 Variables d'Environnement

Ajouter dans votre fichier `.env` ou configuration Render :

```bash
# Activer l'authentification
ENABLE_AUTH=true
ENABLE_RATE_LIMIT=true
ENABLE_ANALYTICS=true

# Configuration existante
DB_HOST=...
DB_PORT=5432
DB_NAME=passbi
DB_USER=...
DB_PASSWORD=...
DB_SSLMODE=require

REDIS_HOST=redis-13600.c339.eu-west-3-1.ec2.cloud.redislabs.com
REDIS_PORT=13600
REDIS_PASSWORD=XQrPtCkQ3Kut00y410VcesVSu5KoJ60o
REDIS_DB=0

API_PORT=8080
```

#### 2.2 Mettre à Jour render.yaml

```yaml
services:
  - type: web
    name: passbi-api
    # ... configuration existante ...
    envVars:
      # ... vars existantes ...

      # Partner System
      - key: ENABLE_AUTH
        value: true
      - key: ENABLE_RATE_LIMIT
        value: true
      - key: ENABLE_ANALYTICS
        value: true
```

---

### Phase 3 : Compilation et Déploiement

#### 3.1 Option A : Remplacer main.go

```bash
# Sauvegarder l'ancien
mv cmd/api/main.go cmd/api/main_old.go

# Utiliser la nouvelle version
mv cmd/api/main_with_auth.go cmd/api/main.go

# Compiler
go build -o bin/passbi-api cmd/api/main.go

# Tester localement
./bin/passbi-api
```

#### 3.2 Option B : Déploiement Progressif

Garder les deux versions et utiliser un flag :

```bash
# Mode sans auth (ancien)
ENABLE_AUTH=false go run cmd/api/main.go

# Mode avec auth (nouveau)
ENABLE_AUTH=true go run cmd/api/main_with_auth.go
```

---

### Phase 4 : Tests

#### 4.1 Test Sans Authentification (si ENABLE_AUTH=false)

```bash
curl http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467
```

#### 4.2 Test Avec Authentification

```bash
# Sans API key (doit échouer)
curl http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467

# Avec API key (doit fonctionner)
curl -H "Authorization: Bearer pk_test_..." \
     http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467
```

#### 4.3 Test du Dashboard

```bash
# Informations partenaire
curl -H "Authorization: Bearer pk_test_..." \
     http://localhost:8080/dashboard/me

# Liste des API keys
curl -H "Authorization: Bearer pk_test_..." \
     http://localhost:8080/dashboard/api-keys

# Créer une nouvelle clé
curl -X POST \
     -H "Authorization: Bearer pk_test_..." \
     -H "Content-Type: application/json" \
     -d '{"name":"New Key","scopes":["read:routes"]}' \
     http://localhost:8080/dashboard/api-keys

# Usage stats
curl -H "Authorization: Bearer pk_test_..." \
     http://localhost:8080/dashboard/usage?days=7

# Quotas
curl -H "Authorization: Bearer pk_test_..." \
     http://localhost:8080/dashboard/quota
```

#### 4.4 Test des Rate Limits

```bash
# Script pour tester les limites
for i in {1..100}; do
    echo "Request $i"
    curl -H "Authorization: Bearer pk_test_..." \
         http://localhost:8080/v2/route-search?from=14.7,-17.4&to=14.8,-17.3
    sleep 0.1
done
```

---

### Phase 5 : Tests avec les SDKs

#### 5.1 Test JavaScript

Créer `test-sdk.js` :

```javascript
const { PassBiClient } = require('./sdks/javascript/passbi-client');

async function test() {
    const client = new PassBiClient('pk_test_...', {
        baseURL: 'http://localhost:8080',
        debug: true
    });

    try {
        // Test route search
        console.log('🔍 Searching routes...');
        const routes = await client.searchRoutes({
            from: '14.7167,-17.4677',
            to: '14.6928,-17.4467'
        });
        console.log('✅ Routes found:', Object.keys(routes.routes));

        // Test nearby stops
        console.log('\n🚏 Finding nearby stops...');
        const stops = await client.findNearbyStops({
            lat: 14.6928,
            lon: -17.4467,
            radius: 500
        });
        console.log('✅ Found', stops.stops.length, 'stops');

        // Test dashboard
        console.log('\n👤 Getting partner info...');
        const info = await client.getPartnerInfo();
        console.log('✅ Partner:', info.name, '- Tier:', info.tier);

        // Test rate limits
        console.log('\n📊 Rate limit info:', client.getRateLimitInfo());

    } catch (error) {
        console.error('❌ Error:', error.message);
    }
}

test();
```

```bash
node test-sdk.js
```

#### 5.2 Test Python

Créer `test_sdk.py` :

```python
from sdks.python.passbi_client import PassBiClient

def test():
    client = PassBiClient(
        'pk_test_...',
        base_url='http://localhost:8080',
        debug=True
    )

    try:
        # Test route search
        print('🔍 Searching routes...')
        routes = client.search_routes('14.7167,-17.4677', '14.6928,-17.4467')
        print(f"✅ Routes found: {list(routes['routes'].keys())}")

        # Test nearby stops
        print('\n🚏 Finding nearby stops...')
        stops = client.find_nearby_stops(14.6928, -17.4467, radius=500)
        print(f"✅ Found {len(stops['stops'])} stops")

        # Test dashboard
        print('\n👤 Getting partner info...')
        info = client.get_partner_info()
        print(f"✅ Partner: {info['name']} - Tier: {info['tier']}")

        # Test rate limits
        print('\n📊 Rate limit info:', client.get_rate_limit_info())

    except Exception as e:
        print(f"❌ Error: {e}")
    finally:
        client.close()

if __name__ == '__main__':
    test()
```

```bash
python test_sdk.py
```

---

## 🔄 Migration Progressive

### Stratégie de Déploiement Sans Interruption

#### Option 1 : Feature Flag

1. Déployer le nouveau code avec `ENABLE_AUTH=false`
2. Créer les comptes partenaires
3. Distribuer les API keys
4. Activer `ENABLE_AUTH=true` progressivement
5. Monitorer et ajuster

#### Option 2 : Dual Mode

1. Garder l'ancien endpoint `/v2/*` public
2. Créer un nouveau endpoint `/v3/*` avec auth
3. Migrer les clients progressivement
4. Déprécier `/v2` après 6 mois

---

## 📊 Monitoring

### Requêtes SQL Utiles

```sql
-- Partenaires actifs
SELECT COUNT(*) FROM partner WHERE status = 'active';

-- API keys actives
SELECT COUNT(*) FROM api_key WHERE is_active = true;

-- Usage du jour
SELECT
    partner_id,
    COUNT(*) as requests,
    AVG(response_time_ms) as avg_response_time
FROM usage_log
WHERE timestamp >= CURRENT_DATE
GROUP BY partner_id
ORDER BY requests DESC;

-- Top 10 partenaires par usage
SELECT
    p.name,
    p.tier,
    COUNT(ul.*) as total_requests
FROM partner p
JOIN usage_log ul ON ul.partner_id = p.id
WHERE ul.timestamp >= NOW() - INTERVAL '30 days'
GROUP BY p.id, p.name, p.tier
ORDER BY total_requests DESC
LIMIT 10;

-- Quotas mensuels
SELECT
    p.name,
    p.tier,
    qu.requests_count,
    p.rate_limit_per_month,
    ROUND(qu.requests_count::numeric / p.rate_limit_per_month * 100, 2) as usage_percent
FROM quota_usage qu
JOIN partner p ON p.id = qu.partner_id
WHERE qu.period_type = 'monthly'
    AND qu.period_start = DATE_TRUNC('month', CURRENT_DATE)
ORDER BY usage_percent DESC;
```

### Alertes Importantes

```sql
-- Partenaires proches de leur limite (>90%)
SELECT
    p.name,
    p.email,
    qu.requests_count,
    p.rate_limit_per_month
FROM quota_usage qu
JOIN partner p ON p.id = qu.partner_id
WHERE qu.period_type = 'monthly'
    AND qu.period_start = DATE_TRUNC('month', CURRENT_DATE)
    AND qu.requests_count::numeric / p.rate_limit_per_month > 0.9;
```

---

## 🐛 Troubleshooting

### Problème : API Keys ne fonctionnent pas

```bash
# Vérifier que les tables existent
psql $DATABASE_URL -c "SELECT * FROM partner LIMIT 1;"

# Vérifier la config
echo $ENABLE_AUTH

# Vérifier le hash de la clé
# Le hash doit correspondre à SHA-256(api_key)
```

### Problème : Rate Limiting ne fonctionne pas

```bash
# Vérifier Redis
redis-cli -h $REDIS_HOST -p $REDIS_PORT -a $REDIS_PASSWORD PING

# Vérifier les clés Redis
redis-cli -h $REDIS_HOST -p $REDIS_PORT -a $REDIS_PASSWORD KEYS "rl:*"
```

### Problème : Analytics non enregistrés

```bash
# Vérifier les logs d'erreur
tail -f logs/app.log | grep "Failed to log"

# Vérifier les données
psql $DATABASE_URL -c "SELECT COUNT(*) FROM usage_log WHERE timestamp >= CURRENT_DATE;"
```

---

## ✅ Checklist de Production

Avant de déployer en production :

- [ ] Migrations exécutées sur la base de production
- [ ] Tables créées et indexes en place
- [ ] Premier partenaire de test créé
- [ ] API key de test générée et fonctionnelle
- [ ] Variables d'environnement configurées
- [ ] Tests d'authentification réussis
- [ ] Tests de rate limiting validés
- [ ] Tests d'analytics vérifiés
- [ ] SDKs testés avec succès
- [ ] Monitoring configuré
- [ ] Alertes en place
- [ ] Documentation distribuée aux partenaires
- [ ] Support prêt à répondre aux questions
- [ ] Plan de rollback préparé

---

## 📞 Support Technique

En cas de problème lors de l'implémentation :

1. Consulter les logs de l'application
2. Vérifier la documentation d'architecture
3. Tester avec curl avant d'utiliser les SDKs
4. Vérifier les variables d'environnement
5. Contacter l'équipe technique

---

**Bonne implémentation ! 🚀**
