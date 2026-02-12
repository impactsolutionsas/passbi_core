# 🚂 Guide de Déploiement sur Railway

Ce guide vous accompagne pour déployer PassBi Core sur Railway avec PostgreSQL + PostGIS et Redis.

## 📋 Prérequis

- Compte Railway : [railway.app](https://railway.app)
- Code source PassBi dans un repository Git (GitHub, GitLab, etc.)
- Carte bancaire (Railway offre $5 de crédit gratuit/mois)

## 🎯 Architecture sur Railway

```
Railway Project
├── PostgreSQL + PostGIS (Database)
├── Redis (Cache)
└── PassBi API (Application Go)
```

## 🚀 Étape 1 : Création du Projet Railway

### 1.1 Créer un nouveau projet

1. Aller sur [railway.app](https://railway.app)
2. Cliquer sur **"New Project"**
3. Choisir **"Empty Project"**
4. Nommer le projet : `passbi-core`

## 📦 Étape 2 : Ajouter PostgreSQL avec PostGIS

### 2.1 Ajouter PostgreSQL

1. Dans votre projet Railway, cliquer sur **"+ New"**
2. Sélectionner **"Database"** → **"PostgreSQL"**
3. Railway va créer automatiquement :
   - Une instance PostgreSQL
   - Variables d'environnement automatiques

### 2.2 Activer l'extension PostGIS

1. Cliquer sur le service **PostgreSQL**
2. Aller dans l'onglet **"Data"** ou **"Connect"**
3. Copier l'URL de connexion (format : `postgresql://user:pass@host:port/db`)
4. Se connecter avec un client PostgreSQL ou via Railway CLI :

```bash
# Installer Railway CLI
npm i -g @railway/cli

# Se connecter
railway login

# Lier le projet
railway link

# Se connecter à PostgreSQL
railway connect postgres
```

5. Exécuter la commande SQL suivante :

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
```

### 2.3 Récupérer les variables d'environnement

Railway génère automatiquement ces variables :
- `DATABASE_URL` : URL complète de connexion
- `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER`, `PGPASSWORD`

## 🔴 Étape 3 : Ajouter Redis

### 3.1 Ajouter Redis

1. Dans votre projet Railway, cliquer sur **"+ New"**
2. Sélectionner **"Database"** → **"Redis"**
3. Railway va créer automatiquement :
   - Une instance Redis
   - Variables d'environnement : `REDIS_URL`, `REDIS_HOST`, `REDIS_PORT`

## 🚢 Étape 4 : Déployer l'Application PassBi

### 4.1 Ajouter le service depuis GitHub

1. Cliquer sur **"+ New"**
2. Sélectionner **"GitHub Repo"**
3. Autoriser Railway à accéder à votre compte GitHub
4. Sélectionner le repository `passbi_core`
5. Railway détecte automatiquement le `Dockerfile`

### 4.2 Configurer les variables d'environnement

1. Cliquer sur le service **PassBi API**
2. Aller dans **"Variables"**
3. Ajouter les variables suivantes :

#### Variables Database (automatiques via Railway)

Railway va automatiquement injecter :
- `${{Postgres.DATABASE_URL}}` : URL complète

Mais PassBi utilise des variables séparées, donc ajouter :

```env
DB_HOST=${{Postgres.PGHOST}}
DB_PORT=${{Postgres.PGPORT}}
DB_NAME=${{Postgres.PGDATABASE}}
DB_USER=${{Postgres.PGUSER}}
DB_PASSWORD=${{Postgres.PGPASSWORD}}
DB_SSLMODE=require
```

#### Variables Redis

```env
REDIS_HOST=${{Redis.REDIS_HOST}}
REDIS_PORT=${{Redis.REDIS_PORT}}
REDIS_PASSWORD=${{Redis.REDIS_PASSWORD}}
REDIS_DB=0
```

#### Variables API

```env
API_PORT=8080
API_READ_TIMEOUT=5s
API_WRITE_TIMEOUT=10s
```

#### Variables Cache

```env
CACHE_TTL=10m
CACHE_MUTEX_TTL=5s
```

#### Variables Routing

```env
MAX_WALK_DISTANCE=500
WALKING_SPEED=1.4
TRANSFER_TIME=180
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
```

### 4.3 Configurer les références entre services

Railway permet de référencer les variables d'autres services :

1. Dans le service **PassBi API**, onglet **Variables**
2. Cliquer sur **"+ Variable Reference"**
3. Sélectionner les variables depuis PostgreSQL et Redis

**Exemple de configuration finale :**

```env
# Database (références au service Postgres)
DB_HOST=${{Postgres.PGHOST}}
DB_PORT=${{Postgres.PGPORT}}
DB_NAME=${{Postgres.PGDATABASE}}
DB_USER=${{Postgres.PGUSER}}
DB_PASSWORD=${{Postgres.PGPASSWORD}}
DB_SSLMODE=require

# Redis (références au service Redis)
REDIS_HOST=${{Redis.REDIS_HOST}}
REDIS_PORT=${{Redis.REDIS_PORT}}
REDIS_PASSWORD=${{Redis.REDIS_PASSWORD}}
REDIS_DB=0

# API Configuration
API_PORT=8080
API_READ_TIMEOUT=5s
API_WRITE_TIMEOUT=10s

# Cache Configuration
CACHE_TTL=10m
CACHE_MUTEX_TTL=5s

# Routing Configuration
MAX_WALK_DISTANCE=500
WALKING_SPEED=1.4
TRANSFER_TIME=180
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
```

### 4.4 Générer un domaine public

1. Dans le service **PassBi API**, onglet **"Settings"**
2. Section **"Networking"**
3. Cliquer sur **"Generate Domain"**
4. Railway va créer une URL publique : `https://passbi-core-production-xxxx.up.railway.app`

## 📊 Étape 5 : Importer les données GTFS

### 5.1 Préparer les fichiers GTFS

1. Placer vos fichiers GTFS dans le dossier `gtfs/` local
2. Compresser en zip : `gtfs.zip`

### 5.2 Option A : Importer via Railway CLI

```bash
# Se connecter au projet
railway link

# Uploader les fichiers GTFS
railway run ./passbi-import -gtfs-dir=/path/to/gtfs
```

### 5.3 Option B : Créer un service d'import temporaire

1. Créer un nouveau service depuis le même repository
2. Dans **"Settings"** → **"Deploy"**
3. Remplacer la commande de démarrage par :
```bash
./passbi-import -gtfs-dir=/app/gtfs
```
4. Uploader les fichiers via Railway volumes ou S3
5. Lancer le service une fois, puis le supprimer

### 5.4 Option C : Import depuis une machine locale

```bash
# Installer railway CLI
npm i -g @railway/cli

# Se connecter au projet
railway login
railway link

# Exporter les variables d'environnement
railway variables

# Importer localement vers Railway DB
./passbi-import -gtfs-dir=./gtfs
```

## 🔧 Étape 6 : Vérification et Tests

### 6.1 Vérifier les logs

1. Cliquer sur le service **PassBi API**
2. Onglet **"Deployments"**
3. Cliquer sur le dernier déploiement
4. Vérifier les logs :

```
✓ Database connected
✓ Redis connected
✓ API server started on :8080
```

### 6.2 Tester l'API

```bash
# Health check
curl https://your-app.up.railway.app/health

# Route search
curl "https://your-app.up.railway.app/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"

# Nearby stops
curl "https://your-app.up.railway.app/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"
```

### 6.3 Surveiller les ressources

1. Railway Dashboard → **"Metrics"**
2. Vérifier :
   - CPU usage
   - Memory usage
   - Network traffic
   - Request count

## 💰 Coûts Railway

### Plan Hobby (Gratuit)

- $5 de crédit gratuit/mois
- Suffisant pour un projet de développement
- Sleep après inactivité

### Plan Developer ($5/mois)

- $5 de crédit inclus
- Pas de sleep
- Meilleure performance

### Estimation des coûts

**Pour PassBi Core :**
- PostgreSQL : ~$2-5/mois
- Redis : ~$1-3/mois
- Application Go : ~$2-5/mois
- **Total estimé : $5-13/mois**

## 🔐 Sécurité

### Variables sensibles

✅ **À FAIRE :**
- Utiliser les variables Railway (chiffrées)
- Activer SSL/TLS (activé par défaut)
- Utiliser des mots de passe forts

❌ **NE PAS FAIRE :**
- Commit des `.env` avec credentials
- Exposer les variables dans les logs
- Utiliser des passwords par défaut

### Activer HTTPS uniquement

Railway fournit HTTPS automatiquement via :
- Certificat SSL gratuit
- Renouvellement automatique
- HTTP redirigé vers HTTPS

## 🚀 CI/CD Automatique

Railway déploie automatiquement à chaque push sur la branche principale.

### Configuration du déploiement automatique

1. Service **PassBi API** → **"Settings"**
2. Section **"Source"**
3. Branche de déploiement : `main` ou `production`
4. Déploiement automatique : **Activé**

### Workflow

```bash
# Développement local
git add .
git commit -m "feat: nouvelle fonctionnalité"
git push origin main

# Railway détecte le push
# → Build automatique
# → Tests (si configurés)
# → Déploiement automatique
# → Health check
```

## 🔄 Rollback

En cas de problème :

1. Aller dans **"Deployments"**
2. Sélectionner un déploiement précédent
3. Cliquer sur **"Redeploy"**

## 📊 Monitoring et Alertes

### Logs en temps réel

```bash
# Via CLI
railway logs

# Via Dashboard
Service → "Deployments" → Dernier déploiement → "View Logs"
```

### Métriques

Railway Dashboard fournit :
- CPU/Memory usage
- Request rate
- Response time
- Error rate

### Alertes (à configurer)

1. Intégrer avec des services externes :
   - Sentry (erreurs)
   - Datadog (métriques)
   - PagerDuty (incidents)

## 🛠️ Commandes Railway CLI Utiles

```bash
# Installer
npm i -g @railway/cli

# Login
railway login

# Lier un projet
railway link

# Variables
railway variables

# Logs
railway logs

# Se connecter à la DB
railway connect postgres

# Se connecter à Redis
railway connect redis

# Déployer manuellement
railway up

# Statut des services
railway status
```

## 🐛 Troubleshooting

### Problème : Database connection failed

```bash
# Vérifier les variables
railway variables

# Tester la connexion PostgreSQL
railway connect postgres
\dx  # Vérifier l'extension PostGIS
```

**Solution :**
- Vérifier que PostGIS est activé : `CREATE EXTENSION postgis;`
- Vérifier les variables `DB_*`

### Problème : Redis connection failed

```bash
# Tester Redis
railway connect redis
PING
```

**Solution :**
- Vérifier les variables `REDIS_*`
- S'assurer que Redis est démarré

### Problème : Out of Memory

**Solution :**
- Augmenter le plan Railway
- Optimiser les requêtes
- Ajouter du caching

### Problème : Slow response times

**Solution :**
- Vérifier les index PostgreSQL
- Optimiser les requêtes
- Augmenter le cache Redis TTL
- Ajouter un CDN

## 📚 Ressources

- [Railway Documentation](https://docs.railway.app)
- [Railway Discord](https://discord.gg/railway)
- [PassBi Documentation](docs/README.md)
- [PostgreSQL + PostGIS](https://postgis.net/)

## ✅ Checklist Finale

- [ ] PostgreSQL créé et PostGIS activé
- [ ] Redis créé
- [ ] Application déployée
- [ ] Variables d'environnement configurées
- [ ] Domaine généré
- [ ] Données GTFS importées
- [ ] Health check ✅
- [ ] Tests API ✅
- [ ] Monitoring activé
- [ ] Backups configurés (optionnel)

---

🎉 **Félicitations ! PassBi est maintenant déployé sur Railway !**

**URL de votre API :** `https://your-project.up.railway.app`

**Next Steps :**
1. Configurer un domaine personnalisé (optionnel)
2. Mettre en place le monitoring
3. Configurer les backups automatiques
4. Ajouter un CDN (Cloudflare)
