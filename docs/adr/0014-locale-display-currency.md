# Display Currency follows locale, not EUR-only

List Price stays USD in the helper snapshot. The plasmoid converts with a daily public FX rate. Default Display Currency is the desktop locale (e.g. DKK in Denmark), with a per-widget ISO override. EUR was only an example from an earlier locale guess, not a product constraint. Failed FX fetch shows USD rather than a guessed rate.
