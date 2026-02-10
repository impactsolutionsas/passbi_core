# 🚀 Guide de Déploiement Production - PassBi Core

Guide complet pour déployer PassBi Core sur Supabase et infrastructure cloud.

## 📋 Prérequis

- [x] Compte Supabase avec projet créé
- [x] Go 1.22+ installé localement
- [x] Redis instance (Upstash, Redis Cloud, ou self-hosted)
- [x] Serveur pour l'API (VPS, Cloud Run, Railway, etc.)

## 🔐 Étape 1: Configuration Supabase

### 1.1 Activer PostGIS

1. Aller sur https://app.supabase.com/project/xlvuggzprjjkzolonbuh/database/extensions
2. Chercher "postgis"
3. Cliquer sur "Enable"

### 1.2 Autoriser votre IP

1. Aller sur https://app.supabase.com/project/xlvuggzprjjkzolonbuh/settings/database
2. Section **"Connection Pooling"**
3. Cliquer sur **"Add your IP address"**
4. Entrer votre IP publique ou `0.0.0.0/0` (⚠️ non recommandé en production)

### 1.3 Obtenir les Credentials

Connection String:
```
postgresql://postgres:[YOUR-PASSWORD]@db.xlvuggzprjjkzolonbuh.supabase.co:5432/postgres
```

Remplacer `[YOUR-PASSWORD]` par votre mot de passe Supabase.

## 🗄️ Étape 2: Appliquer les Migrations

### 2.1 Installer golang-migrate

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### 2.2 Exécuter les Migrations

```bash
# Encoder le mot de passe si il contient des caractères spéciaux
# Exemple: Mounty@890911 → Mounty%40890911

migrate -path migrations \
  -database "postgresql://postgres:YOUR_ENCODED_PASSWORD@db.xlvuggzprjjkzolonbuh.supabase.co:5432/postgres?sslmode=require" \
  up
```

**Vérifier:**
```bash
migrate -path migrations \
  -database "postgresql://postgres:YOUR_ENCODED_PASSWORD@db.xlvuggzprjjkzolonbuh.supabase.co:5432/postgres?sslmode=require" \
  version
```

## 📊 Étape 3: Importer les Données GTFS

### 3.1 Configurer l'Environnement

Créer `.env.production`:

```env
DB_HOST=db.xlvuggzprjjkzolonbuh.supabase.co
DB_PORT=5432
DB_NAME=postgres
DB_USER=postgres
DB_PASSWORD=YOUR_PASSWORD
DB_SSLMODE=require

REDIS_HOST=your-redis-host
REDIS_PORT=6379
REDIS_PASSWORD=your-redis-password

API_PORT=8080
CACHE_TTL=10m
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
```

### 3.2 Importer les Données

```bash
# Charger les variables d'environnement
export $(grep -v '^#' .env.production | xargs)

# Importer Dakar Dem Dikk
go run cmd/importer/main.go \
  --agency-id=dakar_dem_dikk \
  --gtfs=gtfs_folder/gtfs_Dem_Dikk.zip

# Importer AFTU
go run cmd/importer/main.go \
  --agency-id=dakar_aftu \
  --gtfs=gtfs_folder/gtfs_AFTU.zip

# Importer BRT
go run cmd/importer/main.go \
  --agency-id=dakar_brt \
  --gtfs=gtfs_folder/gtfs_BRT.zip

# Importer TER et reconstruire le graphe
go run cmd/importer/main.go \
  --agency-id=dakar_ter \
  --gtfs=gtfs_folder/gtfs_TER.zip \
  --rebuild-graph
```

### 3.3 Vérifier l'Import

```sql
-- Se connecter à Supabase SQL Editor
SELECT
  'stops' as type, COUNT(*)::text as count FROM stop
UNION ALL
SELECT 'routes', COUNT(*)::text FROM route
UNION ALL
SELECT 'nodes', COUNT(*)::text FROM node
UNION ALL
SELECT 'edges', COUNT(*)::text FROM edge;
```

**Résultats attendus:**
- ~1,795 stops
- 134 routes
- ~6,669 nodes
- ~821,060 edges

## 🐳 Étape 4: Containeriser l'API

### 4.1 Créer le Dockerfile

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o passbi-api cmd/api/main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/passbi-api .

EXPOSE 8080

CMD ["./passbi-api"]
```

### 4.2 Créer .dockerignore

```
.git
.env
.env.*
*.log
tmp/
*.md
Dockerfile
.dockerignore
gtfs_folder/
migrations/
```

### 4.3 Build & Push

```bash
# Build
docker build -t passbi-api:latest .

# Tag pour registry
docker tag passbi-api:latest your-registry/passbi-api:latest

# Push
docker push your-registry/passbi-api:latest
```

## 🚀 Étape 5: Déployer l'API

### Option A: Railway

1. Aller sur https://railway.app
2. Créer nouveau projet
3. **Add Service** > **Docker Image**
4. Configurer les variables d'environnement:
   ```
   DB_HOST=db.xlvuggzprjjkzolonbuh.supabase.co
   DB_PORT=5432
   DB_NAME=postgres
   DB_USER=postgres
   DB_PASSWORD=your_password
   DB_SSLMODE=require
   REDIS_HOST=...
   REDIS_PORT=6379
   ```
5. **Deploy**

### Option B: Google Cloud Run

```bash
# Build et push vers Google Container Registry
gcloud builds submit --tag gcr.io/PROJECT_ID/passbi-api

# Deploy
gcloud run deploy passbi-api \
  --image gcr.io/PROJECT_ID/passbi-api \
  --platform managed \
  --region europe-west1 \
  --allow-unauthenticated \
  --set-env-vars="DB_HOST=db.xlvuggzprjjkzolonbuh.supabase.co,DB_PORT=5432,DB_NAME=postgres,DB_USER=postgres,DB_PASSWORD=your_password,DB_SSLMODE=require"
```

### Option C: VPS (Digital Ocean, Linode, etc.)

```bash
# SSH vers le serveur
ssh user@your-server

# Installer Docker
curl -fsSL https://get.docker.com | sh

# Créer .env.production
cat > .env.production << 'EOF'
DB_HOST=db.xlvuggzprjjkzolonbuh.supabase.co
DB_PORT=5432
DB_NAME=postgres
DB_USER=postgres
DB_PASSWORD=your_password
DB_SSLMODE=require
REDIS_HOST=localhost
REDIS_PORT=6379
EOF

# Pull et run
docker pull your-registry/passbi-api:latest

docker run -d \
  --name passbi-api \
  --restart unless-stopped \
  -p 8080:8080 \
  --env-file .env.production \
  your-registry/passbi-api:latest

# Vérifier les logs
docker logs -f passbi-api
```

## 🔴 Étape 6: Configurer Redis

### Option A: Upstash (Recommandé)

1. Aller sur https://upstash.com
2. Créer une database Redis
3. Copier les credentials:
   ```
   REDIS_HOST=your-endpoint.upstash.io
   REDIS_PORT=6379
   REDIS_PASSWORD=your-token
   ```

### Option B: Redis Cloud

1. https://redis.com/try-free/
2. Créer une database
3. Obtenir connection string

### Option C: Self-Hosted

```bash
# Docker
docker run -d \
  --name redis \
  --restart unless-stopped \
  -p 6379:6379 \
  redis:7-alpine redis-server --requirepass your_password
```

## 🔒 Étape 7: Sécurité

### 7.1 Firewall Supabase

Dans Supabase Dashboard:
1. **Database Settings** > **Connection Pooling**
2. **Restrictions**:
   - Limiter aux IPs de vos serveurs
   - Utiliser connection pooler (port 6543)

### 7.2 API Rate Limiting

Ajouter un reverse proxy (Nginx, Caddy) avec rate limiting:

```nginx
limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;

server {
    listen 80;
    server_name api.passbi.com;

    location / {
        limit_req zone=api burst=20 nodelay;
        proxy_pass http://localhost:8080;
    }
}
```

### 7.3 HTTPS

```bash
# Avec Caddy (auto HTTPS)
caddy reverse-proxy --from api.passbi.com --to localhost:8080
```

## 📈 Étape 8: Monitoring

### 8.1 Health Check

```bash
# Cron job pour vérifier la disponibilité
*/5 * * * * curl -f https://api.passbi.com/health || echo "API DOWN" | mail -s "Alert" admin@passbi.com
```

### 8.2 Logs

```bash
# Consulter les logs Docker
docker logs -f --tail=100 passbi-api

# Avec journalctl
journalctl -u passbi-api -f
```

### 8.3 Métriques Redis

```bash
redis-cli -h your-host -a your-password INFO stats
```

## 🔄 Étape 9: CI/CD (GitHub Actions)

Créer `.github/workflows/deploy.yml`:

```yaml
name: Deploy to Production

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Run tests
        run: go test ./...

      - name: Build Docker image
        run: docker build -t passbi-api:${{ github.sha }} .

      - name: Push to registry
        run: |
          echo ${{ secrets.REGISTRY_TOKEN }} | docker login -u ${{ secrets.REGISTRY_USER }} --password-stdin
          docker push passbi-api:${{ github.sha }}

      - name: Deploy to Railway
        run: railway up
        env:
          RAILWAY_TOKEN: ${{ secrets.RAILWAY_TOKEN }}
```

## ✅ Vérification Finale

### Checklist Production

- [ ] PostGIS activé sur Supabase
- [ ] Migrations appliquées
- [ ] Données GTFS importées (4 agences)
- [ ] Redis configuré et accessible
- [ ] API déployée et accessible
- [ ] HTTPS configuré
- [ ] Health check fonctionnel
- [ ] Rate limiting configuré
- [ ] Monitoring en place
- [ ] Backup automatique DB

### Tests de Validation

```bash
# 1. Health check
curl https://api.passbi.com/health

# 2. Route search
curl "https://api.passbi.com/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677"

# 3. Stops nearby
curl "https://api.passbi.com/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"

# 4. Routes list
curl "https://api.passbi.com/v2/routes/list?limit=10"

# 5. Performance test
time curl "https://api.passbi.com/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677"
# Expected: < 500ms
```

## 🆘 Troubleshooting

### Connection to Supabase failed

```bash
# Vérifier IP whitelisting
curl https://api.ipify.org

# Tester connection
psql "postgresql://postgres:PASSWORD@db.xlvuggzprjjkzolonbuh.supabase.co:5432/postgres?sslmode=require"
```

### API returns no routes

```sql
-- Vérifier les données
SELECT COUNT(*) FROM node;
SELECT COUNT(*) FROM edge WHERE type = 'RIDE';

-- Si vides, ré-importer
```

### Redis connection timeout

```bash
# Test Redis
redis-cli -h your-host -a your-pass PING

# Vérifier firewall
telnet your-redis-host 6379
```

## 📞 Support

Pour assistance déploiement:
- Email: devops@passbi.com
- Documentation: https://docs.passbi.com

---

**Bon déploiement! 🚀**
