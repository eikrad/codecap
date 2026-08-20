# One GetSnapshot plus Changed

Several widget instances share one helper and each is bound to one Account Home. A family of getters would duplicate the domain faces in QML. v1 exposes `GetSnapshot(accountHome, timezone, weekStart)` returning JSON for the whole face, and `Changed(accountHome)` when logs or Allowance update. The helper owns polling and file watch. List Price stays USD in the snapshot; Display Currency is a plasmoid concern.
