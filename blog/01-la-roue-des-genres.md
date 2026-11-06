# 01 · La roue des genres

*2026-11-06 · fipindicateur*

## Le constat

- Suivant/précédent zappent en rond sur 13 webradios : un cercle existait déjà
- Ce cercle était l'ordre brut du tableau `stations.All` : arbitraire
- Jugé comme voisinage : 5 accords dissonants sur 13
  - Metal → Sacré français ! : le pire
  - Hip-Hop → Metal, Nouveautés → Reggae, Pop → Cultes, Monde → Nouveautés

## Les candidats

- **A · Éditorial** : paires affines à la main, chaque accord se raconte
  - dispo maintenant, défendable à l'oreille, accords faibles = choix conscients
- **B · Transitions perso** : Held-Karp poids max sur le Markov symétrisé des zaps
  - 13 nœuds : 2¹³ × 13² ≈ 1,4 M d'états, trivial en stdlib Go
  - mesure les habitudes, pas la distance esthétique ; il faut des sessions
- **C · Recoupage d'artistes** : Jaccard sur les programmations Radio France
  - distance objective, indépendante de l'auditeur ; endpoint à vérifier
- **D · Hybride échelonné** : A maintenant · B quand la donnée suit · C plus tard

## La décision

- A puis D : roue éditoriale d'abord, vérification par la donnée ensuite
- Les 6 duos-régions :
  - Guitare : Rock–Metal
  - Électrique : Electro–Hip-Hop (les beats)
  - Caraïbes/Afrique : Reggae–Monde (dub, sound systems)
  - Racines noires : Jazz–Groove
  - Pop actuelle : Pop–Nouveautés
  - Hexagone : Sacré français–Cultes
- FIP à la couture : le hub éclectique entre les deux moitiés
- Accords faibles assumés :
  - Metal–Electro : les extrêmes électriques, l'industriel
  - Groove–Pop : la soul dans la pop
  - Nouveautés–Sacré français : la scène française d'aujourd'hui

```
        FIP
   Cultes     Rock
Sacré Fr      Metal
Nouveautés    Electro
   Pop      Hip-Hop
    Groove--Reggae
      Jazz    Monde
        (→ Jazz)
```

## La mécanique

- `stations.Wheel()` à côté de `stations.All` :
  - `All` : ordre canonique (MetaID, couleurs, stockage), intact
  - `Wheel` : ne façonne que la navigation
- Config persiste des clés, pas des indices : réordonner = zéro migration
- Sous-menu Radios en ordre de roue : le menu se lit comme le cycle next/prev
- Les arêtes Markov sont propres : `startStation` ne journalise que from ≠ to
  - chaque arête = un saut délibéré, pas du bruit de démarrage

## Le garde-fou a payé

- `TestWheelIsPermutation` : même ensemble de clés que `All`, un doublon = fail
- Il a attrapé aussitôt une clé inventée (`monde` : le libellé ; la clé : `world`)
- Un test qui rate sa cible au premier commit, c'est un test qui sert

## La suite (quand la donnée vient)

- Log complet sur le XPS (omon), sauvegarde domovoy ; stats opt-in encore off ici
- Alors : le rapport stats dessine la roue + les accords réels par-dessus
  - paire fréquente mal placée = grande corde qui traverse le cercle
  - le rapport devient l'outil de calibration de la roue, taille d'échantillon affichée
- Recalage Held-Karp si l'écart optimal/éditorial le justifie
- Optionnel : recoupage d'artistes pour une distance externe
