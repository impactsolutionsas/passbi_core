# 🚂 Railway Quickstart - PassBi Core

Guide rapide pour déployer PassBi sur Railway en 10 minutes.

## 📦 Fichiers Préparés

✅ **Tous les fichiers nécessaires sont prêts :**

- `Dockerfile` - Image Docker optimisée
- `.dockerignore` - Exclusions pour le build
- `railway.toml` - Configuration Railway
- `.env.railway` - Template des variables d'environnement
- `scripts/railway-setup.sh` - Script d'installation automatique
- `DEPLOY_RAILWAY.md` - Guide complet détaillé

## 🚀 Déploiement Rapide (10 minutes)

### Étape 1 : Installer Railway CLI (2 min)

```bash
npm install -g @railway/cli
```

### Étape 2 : Lancer le script d'installation (1 min)

```bash
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core
./scripts/railway-setup.sh
```

Le script va :
- Installer Railway CLI si nécessaire
- Vous connecter à Railway
- Créer/lier le projet

### Étape 3 : Ajouter les services sur Railway (3 min)

1. Aller sur [railway.app/dashboard](https://railway.app/dashboard)
2. Ouvrir votre projet `passbi-core`

#### 3.1 Ajouter PostgreSQL

- Cliquer sur **"+ New"**
- Sélectionner **"Database"** → **"PostgreSQL"**
- Attendre la création (30 secondes)

#### 3.2 Activer PostGIS

```bash
railway connect postgres
```

Puis dans le terminal PostgreSQL :

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
\dx  -- Vérifier que PostGIS est installé
\q   -- Quitter
```

#### 3.3 Ajouter Redis

- Cliquer sur **"+ New"**
- Sélectionner **"Database"** → **"Redis"**

#### 3.4 Ajouter l'application

- Cliquer sur **"+ New"**
- Sélectionner **"GitHub Repo"**
- Autoriser l'accès à GitHub
- Sélectionner le repository `passbi_core`

### Étape 4 : Configurer les variables (2 min)

1. Cliquer sur le service **passbi_core**
2. Onglet **"Variables"**
3. Copier les variables depuis `.env.railway` :

```env
# Database
DB_HOST=${{Postgres.PGHOST}}
DB_PORT=${{Postgres.PGPORT}}
DB_NAME=${{Postgres.PGDATABASE}}
DB_USER=${{Postgres.PGUSER}}
DB_PASSWORD=${{Postgres.PGPASSWORD}}
DB_SSLMODE=require

# Redis
REDIS_HOST=${{Redis.REDIS_HOST}}
REDIS_PORT=${{Redis.REDIS_PORT}}
REDIS_PASSWORD=${{Redis.REDIS_PASSWORD}}
REDIS_DB=0

# API Configuration
API_PORT=8080
API_READ_TIMEOUT=5s
API_WRITE_TIMEOUT=10s
CACHE_TTL=10m
CACHE_MUTEX_TTL=5s

# Routing
MAX_WALK_DISTANCE=500
WALKING_SPEED=1.4
TRANSFER_TIME=180
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
```

**Important :** Les références `${{Postgres.XXX}}` et `${{Redis.XXX}}` sont automatiques dans Railway. Il suffit de taper `${{` et Railway proposera l'autocomplétion.

### Étape 5 : Générer un domaine public (1 min)

1. Service **passbi_core** → **"Settings"**
2. Section **"Networking"**
3. Cliquer sur **"Generate Domain"**
4. Copier l'URL : `https://passbi-core-production-xxxx.up.railway.app`

### Étape 6 : Vérifier le déploiement (1 min)

Railway déploie automatiquement. Attendez que le status soit **"Active"** (vert).

```bash
# Vérifier les logs
railway logs

# Tester l'API
curl https://passbi-core-production-xxxx.up.railway.app/health
```

Réponse attendue :

```json
{
  "status": "healthy",
  "timestamp": "2025-02-11T...",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

## 📊 Importer les Données GTFS

### Option 1 : Via Railway CLI (Recommandé)

```bash
# Se connecter au projet
railway link

# Définir les variables localement
railway variables

# Lancer l'import localement (connecté à Railway DB)
go run cmd/importer/main.go -gtfs-dir=./gtfs
```

### Option 2 : Via Service Temporaire

1. Créer un nouveau service depuis le même repo
2. Dans **"Settings"** → **"Deploy"** → **"Custom Start Command"**
3. Définir : `./passbi-import -gtfs-dir=/app/gtfs`
4. Upload les fichiers GTFS
5. Lancer une fois puis supprimer le service

## ✅ Checklist de Vérification

- [ ] PostgreSQL créé et accessible
- [ ] Extension PostGIS activée (`\dx` dans psql)
- [ ] Redis créé et accessible
- [ ] Service PassBi déployé (status "Active")
- [ ] Variables d'environnement configurées
- [ ] Domaine public généré
- [ ] Health check répond `{"status":"healthy"}`
- [ ] Données GTFS importées
- [ ] API route-search fonctionne

## 🧪 Tests Rapides

```bash
# Définir l'URL de votre application
export API_URL="https://passbi-core-production-xxxx.up.railway.app"

# Health check
curl $API_URL/health

# Recherche d'itinéraire (Dakar)
curl "$API_URL/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"

# Arrêts à proximité
curl "$API_URL/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"

# Liste des routes
curl "$API_URL/v2/routes/list?limit=10"
```

## 💰 Coûts Estimés

**Plan Hobby (Gratuit) :**
- $5 de crédit gratuit/mois
- Sleep après inactivité

**Coûts mensuels estimés :**
- PostgreSQL : ~$2-5
- Redis : ~$1-3
- Application : ~$2-5
- **Total : $5-13/mois**

Le crédit gratuit de $5 couvre un usage léger.

## 🔧 Commandes Utiles

```bash
# Voir les logs en temps réel
railway logs -f

# Ouvrir le dashboard
railway open

# Variables d'environnement
railway variables

# Connexion PostgreSQL
railway connect postgres

# Connexion Redis
railway connect redis

# Déployer manuellement
railway up

# Statut des services
railway status
```

## 🐛 Problèmes Courants

### "Database connection failed"

**Solution :**
```bash
railway connect postgres
CREATE EXTENSION IF NOT EXISTS postgis;
```

### "Redis connection timeout"

**Solution :**
- Vérifier que le service Redis est "Active"
- Vérifier les variables `REDIS_*`

### "Build failed"

**Solution :**
- Vérifier que `go.mod` et `go.sum` sont présents
- Vérifier que le `Dockerfile` est à la racine
- Regarder les logs de build

## 📚 Ressources

- **Guide complet** : [DEPLOY_RAILWAY.md](DEPLOY_RAILWAY.md)
- **API Documentation** : [docs/README.md](docs/README.md)
- **Railway Docs** : [docs.railway.app](https://docs.railway.app)
- **Railway Discord** : [discord.gg/railway](https://discord.gg/railway)

## 🎉 Terminé !

Votre API PassBi est maintenant en production sur Railway !

**URL publique :** `https://passbi-core-production-xxxx.up.railway.app`

**Prochaines étapes :**
1. Configurer un domaine personnalisé (optionnel)
2. Mettre en place le monitoring
3. Intégrer l'API dans vos applications (voir [docs/api/examples/](docs/api/examples/))
