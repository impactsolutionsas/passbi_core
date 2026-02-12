# 🚀 Guide de Déploiement sur Render.com

Ce guide vous accompagne pour déployer PassBi Core sur Render avec PostgreSQL + PostGIS et Redis.

## 📋 Prérequis

- Compte Render : [render.com](https://render.com) (Gratuit)
- Code source PassBi dans un repository Git (GitHub, GitLab, Bitbucket)
- **Aucune carte bancaire requise pour le plan gratuit**

## 🎁 Plan Gratuit Render

- ✅ **750 heures/mois** pour les web services
- ✅ **PostgreSQL gratuit** : 1 GB de stockage
- ✅ **Redis gratuit** : 25 MB
- ✅ **SSL automatique** (HTTPS)
- ✅ **Deploy automatique** depuis Git
- ⚠️ **Sleep après 15 min** d'inactivité (réveil en 30s)
- ⚠️ **Pas de carte bancaire** nécessaire pour commencer

## 🚀 Méthode 1 : Déploiement Automatique avec Blueprint (Recommandé)

### Étape 1 : Préparer le repository

1. Assurez-vous que `render.yaml` est à la racine du projet ✅ (Déjà fait)
2. Commit et push vers GitHub :

```bash
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core

git add render.yaml .dockerignore Dockerfile
git commit -m "feat: add Render deployment config"
git push origin main
```

### Étape 2 : Déployer sur Render

1. Aller sur [dashboard.render.com](https://dashboard.render.com)
2. Cliquer sur **"New +"** → **"Blueprint"**
3. Connecter votre compte GitHub
4. Sélectionner le repository `passbi_core`
5. Render détecte automatiquement `render.yaml`
6. Cliquer sur **"Apply"**

✅ **Render va créer automatiquement :**
- PostgreSQL database (`passbi-postgres`)
- Redis instance (`passbi-redis`)
- Web service (`passbi-api`)
- Variables d'environnement liées automatiquement
- Domaine HTTPS public

### Étape 3 : Activer PostGIS

Une fois PostgreSQL créé :

1. Aller sur le service **passbi-postgres**
2. Onglet **"Shell"** ou **"Connect"**
3. Cliquer sur **"PSQL"** pour ouvrir un terminal
4. Exécuter :

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
```

5. Vérifier :

```sql
\dx
```

Vous devriez voir `postgis` dans la liste.

### Étape 4 : Vérifier le déploiement

1. Aller sur le service **passbi-api**
2. Attendre que le status soit **"Live"** (vert)
3. Copier l'URL publique : `https://passbi-api-xxxx.onrender.com`
4. Tester l'API :

```bash
curl https://passbi-api-xxxx.onrender.com/health
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

## 🛠️ Méthode 2 : Déploiement Manuel

Si vous préférez créer les services un par un :

### Étape 1 : Créer PostgreSQL

1. Dashboard Render → **"New +"** → **"PostgreSQL"**
2. Nom : `passbi-postgres`
3. Database : `passbi`
4. User : `passbi`
5. Plan : **Free**
6. PostgreSQL Version : **15**
7. Cliquer sur **"Create Database"**

Attendre 1-2 minutes pour la création.

#### Activer PostGIS

1. Service PostgreSQL → **"Connect"** → **"PSQL"**
2. Exécuter :

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
\dx  -- Vérifier
\q   -- Quitter
```

### Étape 2 : Créer Redis

1. Dashboard → **"New +"** → **"Redis"**
2. Nom : `passbi-redis`
3. Plan : **Free**
4. Maxmemory Policy : **allkeys-lru**
5. Cliquer sur **"Create Redis"**

### Étape 3 : Créer le Web Service

1. Dashboard → **"New +"** → **"Web Service"**
2. **Connect repository** : Autoriser GitHub et sélectionner `passbi_core`
3. Nom : `passbi-api`
4. Runtime : **Docker**
5. Plan : **Free**
6. Branch : `main`

#### Configurer les variables d'environnement

Onglet **"Environment"**, ajouter :

**Database (copier depuis PostgreSQL) :**

```env
DB_HOST=<Copier depuis passbi-postgres → Internal Database URL → Host>
DB_PORT=5432
DB_NAME=passbi
DB_USER=passbi
DB_PASSWORD=<Copier depuis passbi-postgres → Internal Database URL → Password>
DB_SSLMODE=require
```

**Redis (copier depuis Redis) :**

```env
REDIS_HOST=<Copier depuis passbi-redis → Internal Redis URL → Host>
REDIS_PORT=<Copier depuis passbi-redis → Internal Redis URL → Port>
REDIS_PASSWORD=<Copier depuis passbi-redis → Connection String>
REDIS_DB=0
```

**API Configuration :**

```env
API_PORT=8080
API_READ_TIMEOUT=5s
API_WRITE_TIMEOUT=10s
```

**Cache Configuration :**

```env
CACHE_TTL=10m
CACHE_MUTEX_TTL=5s
```

**Routing Configuration :**

```env
MAX_WALK_DISTANCE=500
WALKING_SPEED=1.4
TRANSFER_TIME=180
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
```

7. **Health Check Path** : `/health`
8. Cliquer sur **"Create Web Service"**

Render va build et déployer automatiquement.

## 📊 Importer les Données GTFS

### Option 1 : Via machine locale (Recommandé)

```bash
# Installer PostgreSQL client (si pas déjà installé)
# macOS
brew install postgresql

# Linux
sudo apt-get install postgresql-client

# Récupérer l'External Database URL depuis Render
export DATABASE_URL="postgresql://user:password@host:port/database"

# Définir les variables d'environnement
export DB_HOST=<host>
export DB_PORT=5432
export DB_NAME=passbi
export DB_USER=passbi
export DB_PASSWORD=<password>
export DB_SSLMODE=require

# Lancer l'import
go run cmd/importer/main.go -gtfs-dir=./gtfs
```

### Option 2 : Via Render Shell

1. Build l'importer localement :

```bash
GOOS=linux GOARCH=amd64 go build -o passbi-import cmd/importer/main.go
```

2. Uploader `passbi-import` vers un service de stockage (S3, Dropbox, etc.)
3. Se connecter au service Render via Shell
4. Télécharger et exécuter l'importer

### Option 3 : Créer un Job One-Time

1. Dashboard → **"New +"** → **"Background Worker"**
2. Même configuration que le Web Service
3. **Start Command** : `./passbi-import -gtfs-dir=/app/gtfs`
4. Uploader les fichiers GTFS
5. Lancer une fois puis supprimer le service

## 🔧 Configuration Avancée

### Domaine Personnalisé

1. Service **passbi-api** → **"Settings"**
2. Section **"Custom Domain"**
3. Ajouter votre domaine : `api.passbi.com`
4. Configurer le DNS (CNAME vers Render)

### Variables d'Environnement Sensibles

Render chiffre automatiquement toutes les variables d'environnement.

### Auto-Deploy

1. Service → **"Settings"**
2. **"Auto-Deploy"** : Activé (par défaut)
3. Branch : `main`

Chaque push sur `main` déclenche un déploiement automatique.

### Build Command Personnalisée (Optionnel)

Si nécessaire, dans **Settings** :

```bash
# Build Command (Render utilise le Dockerfile par défaut)
docker build -t passbi-api .
```

## 📈 Monitoring

### Logs en Temps Réel

1. Service → **"Logs"**
2. Voir les logs en temps réel
3. Filtrer par niveau : Info, Warning, Error

### Métriques

1. Service → **"Metrics"**
2. Voir :
   - CPU usage
   - Memory usage
   - Request count
   - Response time

### Alertes

Render envoie des emails automatiquement en cas de :
- Déploiement échoué
- Service down
- Erreurs répétées

## 💰 Coûts (Plan Gratuit)

**Inclus gratuitement :**
- 750h/mois pour le web service (suffisant pour 1 projet)
- PostgreSQL : 1 GB de stockage
- Redis : 25 MB
- SSL/HTTPS automatique
- Bandwidth : 100 GB/mois

**Limitations :**
- Sleep après 15 min d'inactivité (réveil en 30s au premier appel)
- 1 projet gratuit à la fois
- Build partagé (plus lent)

**Upgrade (si besoin) :**
- Plan **Starter** : $7/mois
  - Pas de sleep
  - Build plus rapide
  - Plus de ressources

## 🐛 Troubleshooting

### Problème : Build failed

**Logs à vérifier :**

```
Service → Logs → Build Logs
```

**Solutions courantes :**
- Vérifier que `Dockerfile` est à la racine
- Vérifier que `go.mod` et `go.sum` sont présents
- Vérifier les dépendances dans `vendor/`

### Problème : Database connection failed

**Solution :**

1. Vérifier que PostGIS est activé :

```bash
# Se connecter à PostgreSQL
Service PostgreSQL → Connect → PSQL
\dx
```

2. Vérifier les variables d'environnement :

```
Service API → Environment → Vérifier DB_*
```

3. Tester la connexion :

```bash
# Depuis le Shell du service API
Service → Shell
env | grep DB_
```

### Problème : Redis connection timeout

**Solution :**

1. Vérifier que Redis est "Live" (actif)
2. Vérifier les variables `REDIS_*`
3. Utiliser l'Internal Redis URL (pas l'External)

### Problème : Service en "Sleep"

Le plan gratuit met le service en sleep après 15 min d'inactivité.

**Solutions :**
1. **Upgrade vers plan Starter** ($7/mois)
2. **Utiliser un ping service** :
   - [UptimeRobot](https://uptimerobot.com) (gratuit)
   - [Pingdom](https://www.pingdom.com)
   - Configure un ping toutes les 10 minutes vers `/health`

3. **Créer un cron job** :

```bash
# Sur votre machine locale ou serveur
*/10 * * * * curl https://passbi-api-xxxx.onrender.com/health
```

### Problème : "Out of Memory"

**Solutions :**
1. Optimiser le code (réduire l'utilisation mémoire)
2. Augmenter le cache TTL
3. Upgrade vers un plan payant avec plus de RAM

### Problème : Slow response (réveil)

Lors du premier appel après sleep, le service met ~30s à se réveiller.

**Solutions :**
1. Upgrade vers plan Starter (pas de sleep)
2. Utiliser un ping service (voir ci-dessus)
3. Ajouter un loader côté client

## 🔒 Sécurité

### SSL/HTTPS

✅ Activé automatiquement par Render
✅ Certificat renouvelé automatiquement
✅ HTTP redirigé vers HTTPS

### Variables d'environnement

✅ Chiffrées automatiquement
✅ Jamais exposées dans les logs
✅ Accès restreint

### Database

✅ SSL requis par défaut (`DB_SSLMODE=require`)
✅ Mot de passe généré aléatoirement
✅ Accès restreint au réseau interne Render

### Redis

✅ Mot de passe généré automatiquement
✅ Accès restreint au réseau interne

## 📚 Ressources

- [Render Documentation](https://render.com/docs)
- [Render Community](https://community.render.com)
- [PassBi API Documentation](docs/README.md)
- [PostgreSQL + PostGIS](https://postgis.net/)

## ✅ Checklist Finale

- [ ] Compte Render créé
- [ ] Repository Git connecté
- [ ] PostgreSQL créé
- [ ] PostGIS activé (`\dx` dans psql)
- [ ] Redis créé
- [ ] Web service déployé
- [ ] Variables d'environnement configurées
- [ ] Health check répond `{"status":"healthy"}`
- [ ] Données GTFS importées
- [ ] API route-search fonctionne
- [ ] Domaine personnalisé configuré (optionnel)
- [ ] Ping service configuré (optionnel)

## 🎉 Terminé !

Votre API PassBi est maintenant en production sur Render !

**URL publique :** `https://passbi-api-xxxx.onrender.com`

**Prochaines étapes :**
1. Configurer un domaine personnalisé (optionnel)
2. Mettre en place un ping service (UptimeRobot)
3. Intégrer l'API dans vos applications
4. Consulter [docs/api/examples/](docs/api/examples/) pour les exemples d'intégration

---

**Questions ?**
- [Render Community](https://community.render.com)
- [Documentation PassBi](docs/README.md)
