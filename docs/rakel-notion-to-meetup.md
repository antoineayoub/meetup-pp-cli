# Rakel — création et synchronisation Meetup depuis Notion

## Objectif

Rakel peut transformer une demande simple dans le chat en un ou plusieurs événements Meetup prêts à être publiés, en utilisant **Notion comme source de vérité**.

Principe :

- Rakel crée des **drafts Meetup** par défaut.
- Rakel peut mettre à jour les événements déjà liés à Notion.
- Rakel ne publie publiquement un événement qu’avec une instruction explicite.
- Après création, mise à jour ou publication, Rakel maintient les liens et statuts dans Notion.

---

## Ce que Rakel peut faire

### Créer un événement Meetup depuis une page Notion

À partir d’un event Notion complet, Rakel peut automatiquement :

1. lire les informations canoniques de l’event ;
2. créer le ou les drafts Meetup ;
3. ajouter le titre, la description, la date et la durée ;
4. appliquer le format Online ou Onsite ;
5. ajouter le lien Le Wagon / lien d’accès pour les événements Online ;
6. ajouter les hosts Meetup ;
7. ajouter les topics Meetup ;
8. télécharger et attacher le visuel de couverture ;
9. enregistrer les URLs Meetup retournées dans Notion ;
10. synchroniser les statuts de publication dans Notion.

### Mettre à jour un événement Meetup existant

Quand une URL Meetup est déjà renseignée dans Notion, Rakel peut synchroniser l’event existant au lieu d’en créer un nouveau.

Les champs synchronisés sont :

- titre ;
- description ;
- CTA ;
- date et durée ;
- format ;
- venue ou lien Online ;
- hosts ;
- topics ;
- image ;
- URLs Meetup et statuts Notion.

### Gérer les villes

| Format Notion | Comportement avec `--city auto` |
|---|---|
| `Online` | Crée/met à jour Montréal et Toronto |
| `Onsite` | Crée/met à jour Montréal uniquement |
| `Hybrid` | Montréal sur site + Toronto en ligne, avec le même lien d’inscription canonique |

Rakel peut aussi cibler explicitement Montréal, Toronto ou les deux villes.

### Publier

La publication est séparée de la création :

- sans instruction explicite : Rakel travaille en **draft** ;
- avec instruction explicite de publication : Rakel peut publier et mettre à jour :
  - `On meetup ?`
  - `Published?`
  - `Published at`

---

## Pré-requis dans Notion

Notion est la source de vérité. Chaque page event doit contenir les propriétés suivantes.

### Champs obligatoires

| Propriété Notion | Usage Meetup |
|---|---|
| `Event name` | Titre Meetup |
| `Dates` | Date de début et durée de l’event |
| Description de la page | Corps de la description Meetup |
| `WWW Link` | Lien de registration Le Wagon et lien Online |
| `Format` | `Online`, `Onsite` ou `Hybrid` |
| `Image` | Cover image Meetup |
| `Meetup URL Montreal` | Lien de l’event Montréal ; évite les doublons et permet l’update |
| `Meetup URL Toronto` | Lien de l’event Toronto ; évite les doublons et permet l’update |

### Champs conditionnels

| Propriété Notion | Quand l’utiliser | Format attendu |
|---|---|---|
| `Host ID` | Quand des hosts sont nécessaires | Multi-select contenant les IDs numériques Meetup, ex. `213975606` |
| `Topics` | Quand des topics sont nécessaires | Multi-select : `<topic ID> - <topic name>`, ex. `1521079 - Automation` |
| `Venues` | Obligatoire pour les events `Onsite` | `<venue ID Meetup> - <nom>`, ex. `28150305 - Le Wagon Montréal - New Campus` |

### Champs de callback maintenus par Rakel

| Propriété | Rôle |
|---|---|
| `Meetup URL Montreal` | URL de l’event Montréal créé ou mis à jour |
| `Meetup URL Toronto` | URL de l’event Toronto créé ou mis à jour |
| `On meetup ?` | `true` quand l’event est actif/public |
| `Published?` | `true` quand Meetup confirme un event `ACTIVE` |
| `Published at` | Horodatage de publication confirmée |

---

## Règles de contenu appliquées

### Description Meetup

Rakel utilise `Short event blurb` comme courte introduction (ou le titre en fallback), puis applique la séquence : intro → CTA → description Notion → CTA.

```markdown
<Short event blurb>

Register [Here](https://www.lewagon.com/events/votre-event)

<Description Notion>

Register [Here](https://www.lewagon.com/events/votre-event)
```

Le mot **Here** est le lien. L’URL reste pilotée par `WWW Link` dans Notion.

### Topics

Exemple de valeurs Notion :

```text
1521079 - Automation
488822 - Artificial Intelligence
788002 - Web Technology
```

Rakel extrait les IDs et les transmet à Meetup comme liste de topic IDs.

### Venues Onsite

Exemple :

```text
28150305 - Le Wagon Montréal - New Campus
```

S’il n’y a pas d’ID numérique au début de `Venues`, Rakel considère que la venue n’existe pas encore dans Meetup et bloque la création plutôt que de choisir une adresse au hasard.

---

## Exemple : demande dans le chat

### Demande utilisateur

> Rakel, crée le Meetup pour l’event Notion « Build an AI agent with n8n & Slack ».

### Ce que Rakel fait

1. retrouve ou lit la page Notion correspondant à l’event ;
2. vérifie les pré-requis : titre, dates, description, `WWW Link`, image, format, URLs de callback ;
3. lit les hosts, topics et venue si nécessaire ;
4. détermine les groupes cibles :
   - Online → Montréal + Toronto ;
   - Onsite → Montréal ;
   - Hybrid → Montréal sur site + Toronto en ligne ;
5. vérifie les URLs Meetup déjà enregistrées :
   - URL vide → crée un draft ;
   - URL présente → met à jour l’event lié ;
6. attache l’image et vérifie l’opération ;
7. écrit les URLs Meetup dans Notion ;
8. garde l’event en draft ;
9. renvoie un compte rendu avec les liens, villes, statut et éventuels blockers.

### Exemple de réponse attendue de Rakel

> Les drafts Meetup ont été créés et synchronisés depuis Notion.
>
> - Montréal — Draft — URL : `…`
> - Toronto — Draft — URL : `…`
> - Hosts : Antoine Ayoub, Marie-Gabrielle Ayoub
> - Topics : Automation, Artificial Intelligence, Web Technology
> - Image : attachée
> - Notion : URLs mises à jour
>
> Les drafts sont prêts pour revue. Je n’ai rien publié publiquement.

---

## Commandes techniques

### Créer uniquement depuis Notion

```bash
meetup-pp-cli events create-from-notion \
  --page-id <notion-page-id> \
  --city auto \
  --draft \
  --agent
```

### Créer ou mettre à jour depuis Notion

```bash
meetup-pp-cli events sync-from-notion \
  --page-id <notion-page-id> \
  --city auto \
  --agent
```

### Publier après validation explicite

```bash
meetup-pp-cli events publish-from-notion \
  --page-id <notion-page-id> \
  --city auto \
  --yes \
  --agent
```

---

## Limites et garde-fous

- Pour `Hybrid`, `Venues` doit fournir la venue Meetup de Montréal et `WWW Link` est requis comme lien d’inscription partagé ; Toronto est créé comme événement en ligne.
- Une venue Onsite sans ID Meetup dans `Venues` bloque l’opération.
- Une image absente ou inutilisable bloque l’opération.
- Les URLs de callback Meetup doivent exister dans le schéma Notion.
- Rakel ne publie pas sans instruction explicite.
- Une URL Meetup existante est le lien de référence pour une mise à jour ; elle évite de recréer l’event.
