# ⚡ Guide Démarrage Rapide - PassBi Core

Lancez PassBi Core en moins de 5 minutes!

---

## 🚀 Option 1: Docker Compose (Recommandé)

### Étape 1: Configuration

```bash
# Copier la configuration
cp .env.example .env

# Éditer .env (optionnel pour local)
nano .env
```

### Étape 2: Démarrage

```bash
# Lancer tous les services
./scripts/deploy-local.sh

# Ou manuellement
docker-compose up -d
```

### Étape 3: Vérification

```bash
# Health check
curl http://localhost:8080/health

# Résultat attendu:
# {"status":"healthy","checks":{"database":"ok","redis":"ok"}}
```

### Étape 4: Import GTFS

```bash
# Importer Dakar Dem Dikk
docker-compose exec api ./passbi-import \
  --agency-id=dakar_dem_dikk \
  --gtfs=/app/gtfs_folder/gtfs_Dem_Dikk.zip \
  --rebuild-graph

# Temps estimé: ~1 minute
```

### Étape 5: Tester l'API

```bash
# Recherche d'itinéraire
curl "http://localhost:8080/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677" | jq

# Arrêts à proximité
curl "http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500" | jq

# Liste des routes
curl "http://localhost:8080/v2/routes/list?limit=10" | jq
```

---

## 💻 Option 2: Installation Native

### Prérequis

- Go 1.22+
- PostgreSQL 15+ avec PostGIS
- Redis 7+

### Étape 1: Installation PostgreSQL + PostGIS

**macOS:**
```bash
brew install postgresql@15 postgis
brew services start postgresql@15
```

**Linux (Ubuntu):**
```bash
sudo apt update
sudo apt install postgresql-15 postgresql-15-postgis-3
sudo systemctl start postgresql
```

### Étape 2: Créer la Base de Données

```bash
# Se connecter à PostgreSQL
psql postgres

# Créer DB et activer PostGIS
CREATE DATABASE passbi;
\c passbi
CREATE EXTENSION postgis;
\q
```

### Étape 3: Redis

**macOS:**
```bash
brew install redis
brew services start redis
```

**Linux:**
```bash
sudo apt install redis-server
sudo systemctl start redis
```

### Étape 4: Configuration

```bash
cp .env.example .env
nano .env

# Configurer:
# DB_HOST=localhost
# DB_USER=votre_user
# DB_PASSWORD=votre_password
```

### Étape 5: Migrations

```bash
# Installer migrate
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Appliquer migrations
migrate -path migrations \
  -database "postgresql://user:pass@localhost:5432/passbi?sslmode=disable" \
  up
```

### Étape 6: Import GTFS

```bash
go run cmd/importer/main.go \
  --agency-id=dakar_dem_dikk \
  --gtfs=gtfs_folder/gtfs_Dem_Dikk.zip \
  --rebuild-graph
```

### Étape 7: Lancer l'API

```bash
go run cmd/api/main.go
```

---

## ☁️ Option 3: Production (Supabase)

### Étape 1: Configuration MCP (FAIT ✅)

```bash
# Déjà exécuté
claude mcp add --scope project --transport http supabase \
  "https://mcp.supabase.com/mcp?project_ref=xlvuggzprjjkzolonbuh"
```

**Important**: Redémarrez Claude Code pour charger les outils Supabase MCP.

### Étape 2: Autoriser votre IP

1. Aller sur: https://app.supabase.com/project/xlvuggzprjjkzolonbuh/settings/database
2. Section **"Connection Pooling"**
3. **"Add your IP address"**

### Étape 3: Activer PostGIS

```bash
# Via SQL Editor Supabase
CREATE EXTENSION IF NOT EXISTS postgis;
```

### Étape 4: Migrations Production

```bash
# Configurer .env.production
cp .env.production.example .env.production
nano .env.production

# Appliquer migrations
migrate -path migrations \
  -database "postgresql://postgres:PASSWORD@db.xlvuggzprjjkzolonbuh.supabase.co:5432/postgres?sslmode=require" \
  up
```

### Étape 5: Déploiement

```bash
# Build et deploy
./scripts/deploy-production.sh

# Ou Railway
railway up

# Ou Google Cloud Run
gcloud run deploy passbi-api --image passbi-api:production
```

---

## 🧪 Tests de Validation

### Test 1: Health Check

```bash
curl http://localhost:8080/health
# ✅ {"status":"healthy"}
```

### Test 2: Route Search

```bash
curl "http://localhost:8080/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677"
# ✅ Retourne 4 stratégies (no_transfer, direct, simple, fast)
```

### Test 3: Nearby Stops

```bash
curl "http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"
# ✅ Retourne liste d'arrêts avec distances
```

### Test 4: Routes List

```bash
curl "http://localhost:8080/v2/routes/list?limit=5"
# ✅ Retourne 5 routes
```

### Test 5: Performance

```bash
time curl -s "http://localhost:8080/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677" > /dev/null
# ✅ < 1s (première requête)
# ✅ < 50ms (cached)
```

---

## 🐛 Troubleshooting

### Problème: "Connection refused" sur port 8080

```bash
# Vérifier si le service est en cours
docker-compose ps
# ou
ps aux | grep passbi-api

# Relancer
docker-compose restart api
```

### Problème: "Database connection failed"

```bash
# Vérifier PostgreSQL
docker-compose logs postgres
# ou
psql -U postgres -h localhost -d passbi -c "SELECT 1"

# Vérifier PostGIS
psql -U postgres -h localhost -d passbi -c "SELECT PostGIS_Version()"
```

### Problème: "Redis connection timeout"

```bash
# Tester Redis
redis-cli PING
# ou
docker-compose exec redis redis-cli PING

# Vérifier config
docker-compose logs redis
```

### Problème: "No routes found"

```bash
# Vérifier les données
psql -U postgres -h localhost -d passbi

# Compter les données
SELECT
  'stops' as type, COUNT(*) FROM stop
UNION ALL
SELECT 'routes', COUNT(*) FROM route
UNION ALL
SELECT 'nodes', COUNT(*) FROM node
UNION ALL
SELECT 'edges', COUNT(*) FROM edge;

# Si vides, ré-importer GTFS
```

---

## 📚 Prochaines Étapes

1. **Importer plus d'agences**:
   ```bash
   go run cmd/importer/main.go --agency-id=aftu --gtfs=gtfs_folder/gtfs_AFTU.zip
   go run cmd/importer/main.go --agency-id=brt --gtfs=gtfs_folder/gtfs_BRT.zip --rebuild-graph
   ```

2. **Configurer HTTPS** (production):
   ```bash
   # Avec Caddy
   caddy reverse-proxy --from api.passbi.com --to localhost:8080
   ```

3. **Monitoring**:
   ```bash
   # Logs en temps réel
   docker-compose logs -f api

   # Stats Redis
   redis-cli INFO stats
   ```

4. **Backups**:
   ```bash
   # Backup PostgreSQL
   docker-compose exec postgres pg_dump -U passbi_user passbi > backup.sql
   ```

---

## 🎉 Félicitations!

PassBi Core est maintenant opérationnel!

- 📖 **Documentation complète**: README.md
- 🚀 **Guide déploiement**: DEPLOYMENT.md
- 📊 **État du projet**: STATUS.md

**Support**: dev@passbi.com
