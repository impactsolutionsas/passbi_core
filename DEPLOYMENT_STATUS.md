# 📋 État du Déploiement PassBI sur Render

**Date**: 2026-02-12
**Status**: En cours - Prêt pour les migrations et l'import de données

---

## ✅ Ce qui a été fait

### 1. Code Préparé et Pushé
- ✅ Tous les changements committés (commit: `75f0ef5`)
- ✅ Code pushé sur GitHub `impactsolutionsas/passbi_core` branche `dev`
- ✅ 24 fichiers ajoutés (6261 nouvelles lignes)
- ✅ Partner API System complet
- ✅ Support hybride PostGIS (prod) + Haversine (local)
- ✅ Migrations préparées
- ✅ SDKs JavaScript et Python

### 2. Base de Données Créée sur Render
- ✅ PostgreSQL 15 créé sur Render
- ✅ Credentials obtenus

---

## 🔑 Credentials et URLs

### Base de Données PostgreSQL (Render)

**Internal URL** (pour connexion depuis services Render):
```
postgresql://passbidev:EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip@dpg-d66r4f8gjchc738fkom0-a/passbidb
```

**External URL** (pour connexion depuis machine locale):
```
postgresql://passbidev:EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip@dpg-d66r4f8gjchc738fkom0-a.frankfurt-postgres.render.com/passbidb?sslmode=require
```

**Détails**:
- Host (internal): `dpg-d66r4f8gjchc738fkom0-a`
- Host (external): `dpg-d66r4f8gjchc738fkom0-a.frankfurt-postgres.render.com`
- Port: `5432`
- Database: `passbidb`
- User: `passbidev`
- Password: `EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip`
- SSL Mode: `require`

### Redis (Redis Labs - Externe)

**Configuration dans render.yaml**:
- Host: `redis-13600.c339.eu-west-3-1.ec2.cloud.redislabs.com`
- Port: `13600`
- Password: `XQrPtCkQ3Kut00y410VcesVSu5KoJ60o`
- DB: `0`

### Repository GitHub
- URL: `https://github.com/impactsolutionsas/passbi_core`
- Branche actuelle: `dev` (⚠️ render.yaml utilise `main`)

---

## ⏳ Prochaines Étapes à Faire

### Étape 1: Activer PostGIS (URGENT - À faire en premier)

**Via l'interface Render**:
1. Aller sur https://dashboard.render.com
2. Trouver le service PostgreSQL
3. Cliquer sur **"Connect"** → **"PSQL"**
4. Exécuter dans la console web:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
\dx
SELECT PostGIS_Version();
```

**Résultat attendu**: Version PostGIS ~3.x affichée

---

### Étape 2: Exécuter les Migrations

**Prérequis**: PostgreSQL client installé (`psql`)

**Sur macOS**:
```bash
brew install postgresql@15
```

**Sur Ubuntu/Debian**:
```bash
sudo apt-get install postgresql-client
```

**Commandes d'exécution**:

```bash
# Se placer dans le projet
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core

# OU sur le nouvel ordinateur, cloner d'abord:
# git clone https://github.com/impactsolutionsas/passbi_core.git
# cd passbi_core
# git checkout dev

# Définir l'URL de connexion
export DATABASE_URL="postgresql://passbidev:EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip@dpg-d66r4f8gjchc738fkom0-a.frankfurt-postgres.render.com/passbidb?sslmode=require"

# Migration 1: Schéma initial avec PostGIS
psql $DATABASE_URL -f migrations/001_initial_schema.up.sql

# Vérifier les tables créées
psql $DATABASE_URL -c "\dt"
# Attendu: stop, route, node, edge, import_log

# Migration 2: Système Partner API
psql $DATABASE_URL -f migrations/002_partner_system.up.sql

# Vérifier les nouvelles tables
psql $DATABASE_URL -c "\dt"
# Attendu: partner, api_key, usage_log, quota_usage, tier_config

# Vérifier les tiers de configuration
psql $DATABASE_URL -c "SELECT tier, rate_limit_per_day, price_per_month FROM tier_config;"
```

**Résultat attendu**: Toutes les tables créées sans erreur

---

### Étape 3: Importer les Données GTFS

**Prérequis**:
- Go installé (pour compiler l'importer)
- Fichiers GTFS dans `gtfs_folder/`

**Commandes d'import**:

```bash
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core

# Recompiler l'importer
go build -o bin/passbi-import cmd/importer/main.go

# Configurer les variables d'environnement
export DB_HOST="dpg-d66r4f8gjchc738fkom0-a.frankfurt-postgres.render.com"
export DB_PORT=5432
export DB_NAME=passbidb
export DB_USER=passbidev
export DB_PASSWORD="EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip"
export DB_SSLMODE=require

# Vérifier la connexion
psql "postgresql://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=require" -c "SELECT version();"

# Import 1: TER (Train Express Régional)
echo "🚆 Import TER..."
./bin/passbi-import \
  --agency-id=dakar_ter \
  --gtfs=gtfs_folder/gtfs_TER.zip \
  --rebuild-graph

# Import 2: BRT (Bus Rapid Transit)
echo "🚍 Import BRT..."
./bin/passbi-import \
  --agency-id=dakar_brt \
  --gtfs=gtfs_folder/gtfs_BRT.zip \
  --rebuild-graph

# Import 3: Dem Dikk
echo "🚌 Import Dem Dikk..."
./bin/passbi-import \
  --agency-id=dakar_dem_dikk \
  --gtfs=gtfs_folder/gtfs_Dem_Dikk.zip \
  --rebuild-graph

# Import 4: AFTU
echo "🚐 Import AFTU..."
./bin/passbi-import \
  --agency-id=dakar_aftu \
  --gtfs=gtfs_folder/gtfs_AFTU.zip \
  --rebuild-graph

# Vérifier les statistiques finales
psql "postgresql://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=require" -c "
SELECT
  'Agencies' as entity, COUNT(DISTINCT agency_id) as count FROM stop
UNION ALL SELECT 'Stops', COUNT(*) FROM stop
UNION ALL SELECT 'Routes', COUNT(*) FROM route
UNION ALL SELECT 'Nodes', COUNT(*) FROM node
UNION ALL SELECT 'Edges', COUNT(*) FROM edge;
"
```

**Résultats attendus**:
- Agencies: 4
- Stops: ~2,800-3,000
- Routes: ~130-150
- Nodes: ~9,000-10,000
- Edges: ~1,200,000-1,300,000

**Durée estimée**: 15-30 minutes

---

### Étape 4: Déployer l'API sur Render

**Option A: Via Blueprint (Recommandé mais nécessite mise à jour)**

⚠️ **Attention**: Le render.yaml pointe vers la branche `main`, mais le code est sur `dev`

**Choix à faire**:
1. **Merger dev → main** puis utiliser Blueprint
2. **Modifier render.yaml** pour utiliser branche `dev`

**Si vous choisissez Option 1 (Merger vers main)**:

```bash
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core

# Merger dev vers main
git checkout main
git merge dev
git push origin main

# Puis créer le Blueprint sur Render Dashboard
```

**Si vous choisissez Option 2 (Modifier render.yaml)**:

```bash
# Éditer render.yaml, changer ligne 12:
# branch: dev

git add render.yaml
git commit -m "fix: use internal hostname for database connection"
git push origin dev
```

**Créer le Blueprint**:
1. Aller sur https://dashboard.render.com
2. Cliquer **"New +"** → **"Blueprint"**
3. Connecter GitHub → Sélectionner `passbi_core`
4. Branche: `main` (ou `dev` si modifié)
5. Render détecte `render.yaml`
6. **IMPORTANT**: Avant de cliquer "Apply", vérifier que le service PostgreSQL pointe vers votre base existante

**Option B: Créer le service API manuellement**

1. Dashboard Render → **"New +"** → **"Web Service"**
2. Connecter repository `passbi_core`
3. Nom: `passbi-api`
4. Runtime: **Docker**
5. Branch: `dev` (ou `main`)
6. Plan: **Free**

**Variables d'environnement à ajouter**:

```env
# Database (copier depuis votre base existante)
DB_HOST=dpg-d66r4f8gjchc738fkom0-a
DB_PORT=5432
DB_NAME=passbidb
DB_USER=passbidev
DB_PASSWORD=EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip
DB_SSLMODE=require

# Redis (Redis Labs externe)
REDIS_HOST=redis-13600.c339.eu-west-3-1.ec2.cloud.redislabs.com
REDIS_PORT=13600
REDIS_PASSWORD=XQrPtCkQ3Kut00y410VcesVSu5KoJ60o
REDIS_DB=0

# API Configuration
API_PORT=8080
API_READ_TIMEOUT=5s
API_WRITE_TIMEOUT=10s

# Cache
CACHE_TTL=10m
CACHE_MUTEX_TTL=5s

# Routing
MAX_WALK_DISTANCE=500
WALKING_SPEED=1.4
TRANSFER_TIME=180
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
```

7. Health Check Path: `/health`
8. Cliquer **"Create Web Service"**

---

### Étape 5: Tester l'API en Production

**Une fois l'API déployée**:

```bash
# Récupérer l'URL publique depuis Render Dashboard
export API_URL="https://passbi-api-xxxx.onrender.com"

# Test 1: Health Check
curl -s $API_URL/health | jq .

# Test 2: Nearby Stops
curl -s "$API_URL/v2/stops/nearby?lat=14.6757028&lon=-17.4331138889&radius=1000" | jq '.stops | length'

# Test 3: Route Search
curl -s "$API_URL/v2/route-search?from=14.6757028,-17.4331138889&to=14.6983722,-17.4414194444444" | jq '.routes.fast'

# Test 4: Routes List
curl -s "$API_URL/v2/routes/list" | jq '.routes | length'
```

---

## 🎯 Checklist Complète

- [x] Code préparé et committé
- [x] Code pushé sur GitHub (branche dev)
- [x] Base de données PostgreSQL créée sur Render
- [ ] PostGIS activé sur la base de données
- [ ] Migrations exécutées (001 et 002)
- [ ] Données GTFS importées (4 agences)
- [ ] Service API déployé sur Render
- [ ] Health check retourne `{"status":"healthy"}`
- [ ] Tests endpoints passent avec succès
- [ ] Métriques vérifiées (CPU, Memory, Logs)

---

## 📂 Fichiers Importants

### Sur le projet local
- `migrations/001_initial_schema.up.sql` - Schéma PostGIS
- `migrations/002_partner_system.up.sql` - Partner API
- `render.yaml` - Configuration Render Blueprint
- `Dockerfile` - Build multi-stage
- `gtfs_folder/` - Données GTFS (4 fichiers .zip)

### Documentation complète
- `DEPLOY_RENDER.md` - Guide détaillé (463 lignes)
- `docs/TEST_RESULTS.md` - Résultats des tests locaux
- `docs/IMPLEMENTATION_GUIDE.md` - Guide d'implémentation

---

## 🛠️ Outils Nécessaires

Pour continuer sur un autre ordinateur:

```bash
# macOS
brew install postgresql@15  # Pour psql
brew install go            # Pour compiler l'importer
brew install jq            # Pour formater les réponses JSON

# Ubuntu/Debian
sudo apt-get update
sudo apt-get install postgresql-client
sudo apt-get install golang-go
sudo apt-get install jq

# Vérifier les versions
psql --version      # >= 15
go version          # >= 1.23
jq --version
```

---

## 🚨 Points d'Attention

### 1. Branche Git
⚠️ Code actuellement sur `dev`, mais `render.yaml` pointe vers `main`

**Solution**: Choisir entre merger vers main ou modifier render.yaml

### 2. Nom de la Base de Données
⚠️ render.yaml crée une base `passbi`, mais vous avez `passbidb`

**Solution**: Le service API manuel avec les bonnes variables d'env contourne ce problème

### 3. Redis External vs Internal
⚠️ render.yaml utilise Redis Labs externe (mot de passe exposé)

**Alternative**: Créer un service Redis sur Render (plus sécurisé)

---

## 📞 Support et Ressources

- **Documentation Render**: https://render.com/docs
- **Render Community**: https://community.render.com
- **Repository GitHub**: https://github.com/impactsolutionsas/passbi_core
- **PostGIS Docs**: https://postgis.net/

---

## 💡 Commandes Rapides de Vérification

```bash
# Vérifier la connexion à la base
psql "postgresql://passbidev:EUIiKWVrCbMOf2W5XW8udFY14HOj4Zip@dpg-d66r4f8gjchc738fkom0-a.frankfurt-postgres.render.com/passbidb?sslmode=require" -c "SELECT version();"

# Lister les tables
psql "$DATABASE_URL" -c "\dt"

# Compter les données
psql "$DATABASE_URL" -c "
SELECT
  (SELECT COUNT(*) FROM stop) as stops,
  (SELECT COUNT(*) FROM route) as routes,
  (SELECT COUNT(*) FROM node) as nodes,
  (SELECT COUNT(*) FROM edge) as edges;
"

# Vérifier PostGIS
psql "$DATABASE_URL" -c "SELECT PostGIS_Version();"
```

---

## 🎉 Une fois terminé

Votre API PassBI sera accessible publiquement:
- **URL**: `https://passbi-api-xxxx.onrender.com`
- **Health**: `https://passbi-api-xxxx.onrender.com/health`
- **Docs**: Documentation complète dans `docs/`

**Prochaines étapes après déploiement**:
1. Configurer UptimeRobot (éviter sleep gratuit)
2. Domaine personnalisé (optionnel)
3. Créer premiers partenaires et clés API
4. Activer rate limiting et analytics

---

**Status actuel**: ✅ Prêt pour migrations et import GTFS
**Dernière mise à jour**: 2026-02-12 10:30
