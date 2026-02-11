# 🚀 Déploiement Production - Guide Pas à Pas

**Suivez ces étapes dans l'ordre pour déployer PassBi Core sur Supabase**

---

## ✅ Étape 1: Autoriser votre IP Supabase (2 min)

### Actions à effectuer:

1. **Obtenir votre IP publique**:
   ```bash
   curl https://api.ipify.org
   ```
   Notez l'adresse IP affichée (ex: `41.82.123.45`)

2. **Aller sur Supabase Dashboard**:
   - URL: https://app.supabase.com/project/xlvuggzprjjkzolonbuh/settings/database
   - Section: **"Connection Pooling"**

3. **Ajouter votre IP**:
   - Cliquer sur **"Add your IP address"** ou **"Configure network restrictions"**
   - Entrer votre IP ou `0.0.0.0/0` (⚠️ moins sécurisé mais fonctionne partout)
   - Sauvegarder

### Vérification:
```bash
# Test de connexion (remplacer PASSWORD par votre mot de passe)
psql "postgresql://postgres:PASSWORD@db.xlvuggzprjjkzolonbuh.supabase.co:5432/postgres?sslmode=require" -c "SELECT 1;"
```

**✅ Si ça affiche "1", c'est bon! Passez à l'étape 2.**

---

## ✅ Étape 2: Configurer .env.production (1 min)

### Actions à effectuer:

```bash
# Le fichier .env.production existe déjà
nano .env.production
```

### Modifier cette ligne:
```env
DB_PASSWORD=your_password_here
```

### Par:
```env
DB_PASSWORD=Mounty@890911
```

### Sauvegarder: `Ctrl+X`, `Y`, `Enter`

---

## ✅ Étape 3: Configurer Supabase (2 min)

### Exécuter le script de configuration:

```bash
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core

# Configurer PostGIS et appliquer migrations
./scripts/setup-supabase.sh
```

### Ce script va:
1. ✅ Tester la connexion Supabase
2. ✅ Activer PostGIS extension
3. ✅ Appliquer les migrations (créer tables)
4. ✅ Afficher les statistiques

**Temps estimé**: 30 secondes

---

## ✅ Étape 4: Importer les Données GTFS (5-10 min)

### Option A: Import Interactif (Recommandé)

```bash
./scripts/import-gtfs-to-supabase.sh
```

Le script vous demandera quelles agences importer:
- **Option 5**: Toutes les agences (recommandé)

### Option B: Import Manuel

```bash
# Charger les variables
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

# Importer TER et rebuild graph
go run cmd/importer/main.go \
  --agency-id=dakar_ter \
  --gtfs=gtfs_folder/gtfs_TER.zip \
  --rebuild-graph
```

### Résultat attendu:
```
✅ Stops: ~1,795
✅ Routes: 134
✅ Nodes: ~6,669
✅ Edges: ~821,060
```

---

## ✅ Étape 5: Tester Localement avec Supabase (1 min)

### Lancer l'API en local connectée à Supabase:

```bash
# Charger les variables de production
export $(grep -v '^#' .env.production | xargs)

# Lancer l'API
go run cmd/api/main.go
```

### Tester les endpoints:

```bash
# Nouveau terminal

# 1. Health check
curl http://localhost:8080/health

# 2. Route search
curl "http://localhost:8080/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677" | jq

# 3. Stops nearby
curl "http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500" | jq

# 4. Routes list
curl "http://localhost:8080/v2/routes/list?limit=10" | jq
```

**Si tous les tests passent, l'API fonctionne avec Supabase! 🎉**

---

## ✅ Étape 6: Déployer l'API en Production

### Option A: Railway (Le plus simple)

1. **Installer Railway CLI**:
   ```bash
   npm install -g @railway/cli
   ```

2. **Login**:
   ```bash
   railway login
   ```

3. **Créer projet**:
   ```bash
   railway init
   ```

4. **Configurer variables**:
   ```bash
   railway variables set DB_HOST=db.xlvuggzprjjkzolonbuh.supabase.co
   railway variables set DB_PORT=5432
   railway variables set DB_NAME=postgres
   railway variables set DB_USER=postgres
   railway variables set DB_PASSWORD=Mounty@890911
   railway variables set DB_SSLMODE=require
   railway variables set REDIS_HOST=localhost
   railway variables set REDIS_PORT=6379
   ```

5. **Déployer**:
   ```bash
   railway up
   ```

### Option B: Docker + VPS

```bash
# Build l'image
docker build -t passbi-api:production .

# Run avec Supabase
docker run -d \
  --name passbi-api \
  -p 8080:8080 \
  --restart unless-stopped \
  --env-file .env.production \
  passbi-api:production

# Vérifier
docker logs -f passbi-api
```

### Option C: Google Cloud Run

```bash
# Build et push
gcloud builds submit --tag gcr.io/YOUR_PROJECT_ID/passbi-api

# Deploy
gcloud run deploy passbi-api \
  --image gcr.io/YOUR_PROJECT_ID/passbi-api \
  --platform managed \
  --region europe-west1 \
  --allow-unauthenticated \
  --set-env-vars="$(cat .env.production | tr '\n' ',' | sed 's/,$//')"
```

---

## ✅ Étape 7: Configurer Redis Cloud (Optionnel)

### Option A: Upstash (Gratuit)

1. Aller sur https://upstash.com
2. Créer une database Redis
3. Copier les credentials
4. Mettre à jour `.env.production`:
   ```env
   REDIS_HOST=your-endpoint.upstash.io
   REDIS_PORT=6379
   REDIS_PASSWORD=your-token
   ```

### Option B: Redis Cloud

1. https://redis.com/try-free/
2. Créer database
3. Copier connection string
4. Mettre à jour `.env.production`

---

## ✅ Étape 8: Tests Production

### Une fois déployé, tester:

```bash
# Remplacer YOUR_DOMAIN par votre domaine Railway/Cloud Run
API_URL="https://your-app.railway.app"

# Health check
curl $API_URL/health

# Route search
curl "$API_URL/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677"

# Performance test
time curl -s "$API_URL/v2/route-search?from=14.6928,-17.4467&to=14.7167,-17.4677" > /dev/null
```

---

## 🎉 Félicitations!

Votre API PassBi Core est maintenant en production sur Supabase!

### Statistiques:
- ✅ 134 routes (4 agences)
- ✅ ~1,795 arrêts
- ✅ 4 stratégies de routage
- ✅ Performance: <500ms P95
- ✅ Cache Redis actif

### Monitoring:

```bash
# Logs Railway
railway logs

# Stats Supabase
# → https://app.supabase.com/project/xlvuggzprjjkzolonbuh/database/tables

# Redis stats (si Upstash)
redis-cli -h your-host -a your-pass INFO stats
```

---

## 🆘 Besoin d'aide?

- **Connection Supabase échoue**: Vérifiez IP whitelist
- **Import GTFS lent**: Normal, ~5-10 min pour toutes les agences
- **API 500 error**: Vérifiez `docker logs` ou `railway logs`
- **No routes found**: Vérifiez que le graphe est bien reconstruit

**Support**: dev@passbi.com
