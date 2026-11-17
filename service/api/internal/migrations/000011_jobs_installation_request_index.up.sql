-- IR839: the jobs of an installation request are looked up by installation_request_id (the request list's linked jobs,
-- one subquery per request row, and the open-job count when HQ creates an installation job). PostgreSQL does not index
-- the referencing columns of a foreign key, so each lookup read the whole jobs table.
CREATE INDEX IF NOT EXISTS jobs_installation_request ON maintenance.jobs (installation_request_id)
  WHERE installation_request_id IS NOT NULL;
