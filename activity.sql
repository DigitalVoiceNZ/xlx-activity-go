-- added indexes to speed dashboard stats analysis

-- core time-range index (speeds up All Modules queries and activity.go
--   startup)
CREATE INDEX idx_activity_ts ON activity (ts);

-- module-scoped time-range index (speeds up Single Module queries
--  dropdown selection)
CREATE INDEX idx_activity_module_ts ON activity (module, ts);

-- call-grouped time-range index (speeds up GROUP BY call queries:
--  activity & kerchunks)
CREATE INDEX idx_activity_call_ts ON activity (call, ts);

ANALYZE activity;

