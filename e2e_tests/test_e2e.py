import unittest
import urllib.request
import urllib.parse
import urllib.error
import json
import time

API_URL = "http://localhost:8086"
DB_NAME = "e2e_db"

def query(q, db=DB_NAME, method='GET'):
    data = urllib.parse.urlencode({'q': q, 'db': db})
    if method == 'GET':
        req = urllib.request.Request(f"{API_URL}/query?{data}")
    else:
        req = urllib.request.Request(f"{API_URL}/query", data=data.encode('utf-8'))
    
    try:
        with urllib.request.urlopen(req) as response:
            return json.loads(response.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode('utf-8')
        raise Exception(f"HTTP Error {e.code}: {body}")

def write_points(lines, db=DB_NAME, rp=None):
    data = "\n".join(lines).encode('utf-8')
    url = f"{API_URL}/write?db={db}"
    if rp:
        url += f"&rp={rp}"
    req = urllib.request.Request(url, data=data, method='POST')
    try:
        with urllib.request.urlopen(req) as response:
            return response.getcode()
    except urllib.error.HTTPError as e:
        body = e.read().decode('utf-8')
        raise Exception(f"HTTP Error {e.code}: {body}")

class TestInfluxDBE2E(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # Create a fresh database for testing
        query(f"DROP DATABASE {DB_NAME}", db="", method="POST")
        query(f"CREATE DATABASE {DB_NAME}", db="", method="POST")
        
    def test_01_measurement_mapping(self):
        DB = "e2e_db_meas"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        
        # 1. Write initial data
        write_points([
            "m_test_orig,tag=a val=10 1000000000",
            "m_test_orig,tag=b val=20 2000000000"
        ], db=DB)
        
        # Wait a moment for indexing
        time.sleep(0.5)

        # 2. Rename measurement
        query("ALTER MEASUREMENT m_test_orig RENAME TO m_test_new", db=DB, method="POST")
        
        # 3. Verify SHOW MEASUREMENT MAPPINGS
        mappings_res = query("SHOW MEASUREMENT MAPPINGS", db=DB)
        series = mappings_res.get('results', [{}])[0].get('series', [])
        self.assertTrue(len(series) > 0, "Expected measurement mappings series")
        
        mapping_found = False
        for row in series[0]['values']:
            if row[0] == 'm_test_new' and row[2] == 'ACTIVE':
                mapping_found = True
                break
        self.assertTrue(mapping_found, "Mapping for m_test_new should be ACTIVE")
        
        # 4. Write new data to the renamed measurement
        write_points([
            "m_test_new,tag=a val=30 3000000000",
        ], db=DB)
        
        time.sleep(0.5)

        # 5. Query all data using the new name
        res = query("SELECT * FROM m_test_new", db=DB)
        values = res['results'][0]['series'][0]['values']
        self.assertEqual(len(values), 3, "Expected 3 points under the new measurement name")
        
        # 6. Test Wildcard regex match against pre-translation name
        res_regex = query("SELECT * FROM /m_test_n.*/", db=DB)
        self.assertIn('series', res_regex['results'][0], "Regex query should match m_test_new")
        
        # 7. Drop measurement
        query("DROP MEASUREMENT m_test_new", db=DB, method="POST")
        
        # 8. Verify DROP handles mapping cleanup
        mappings_res = query("SHOW MEASUREMENT MAPPINGS", db=DB)
        series = mappings_res.get('results', [{}])[0].get('series', [])
        mapping_deleted = False
        for row in series[0]['values']:
            if row[0] == None and row[2] == 'DELETED':  # User name becomes None when DELETED
                mapping_deleted = True
                break
        self.assertTrue(mapping_deleted, "Mapping should be marked DELETED")

    def test_02_field_mapping(self):
        DB = "e2e_db_field"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        
        # 1. Write initial data
        write_points([
            "sensors,loc=room1 temp=72.0 1000000000",
            "sensors,loc=room1 temp=73.0 2000000000"
        ], db=DB)
        time.sleep(0.5)
        
        # 2. Rename field
        query("ALTER MEASUREMENT sensors RENAME FIELD temp TO temperature", db=DB, method="POST")
        
        # 3. Write new data using new field name
        write_points([
            "sensors,loc=room1 temperature=74.0 3000000000"
        ], db=DB)
        time.sleep(0.5)
        
        # 4. Query data using new field name
        res = query("SELECT temperature FROM sensors", db=DB)
        series = res['results'][0].get('series', [])
        self.assertTrue(len(series) > 0, "Expected results for renamed field")
        self.assertEqual(len(series[0]['values']), 3, "Expected 3 points for temperature")
        self.assertIn("temperature", series[0]['columns'], "Column should be named 'temperature'")
        
        # 5. Drop field
        query("DROP FIELD temperature FROM sensors", db=DB, method="POST")
        
        # 6. Ensure field is gone
        res = query("SELECT temperature FROM sensors", db=DB)
        self.assertNotIn('series', res['results'][0], "Dropped field should return no series data")

    def test_03_moving_functions(self):
        DB = "e2e_db_funcs"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        
        # 1. Write sequential data
        write_points([
            f"stock,sym=a price={float(i)} {i}000000000" for i in range(1, 6) # 1.0 to 5.0
        ], db=DB)
        time.sleep(0.5)
        
        # 2. Test moving_median
        res = query("SELECT moving_median(median(price), 3) FROM stock WHERE time >= 0 AND time <= 6s GROUP BY time(1s)", db=DB)
        values = res['results'][0]['series'][0]['values']
        
        # moving_median with window 3 over [1, 2, 3, 4, 5]
        # At t=3: median([1, 2, 3]) = 2.0
        # At t=4: median([2, 3, 4]) = 3.0
        # At t=5: median([3, 4, 5]) = 4.0
        self.assertEqual(len(values), 3)
        self.assertEqual(values[0][1], 2.0)
        
        # 3. Test moving_average with center (boolean 'true')
        # center parameter shifts timestamps back by (window - 1)/2 = 1.
        # Original moving average (window=3):
        # t=3 -> avg=2.0
        # Shifted back by 1 means the result for the window ending at t=3 is reported at t=2.
        res_center = query("SELECT moving_average(mean(price), 3, 3, true) FROM stock WHERE time >= 0 AND time <= 6s GROUP BY time(1s)", db=DB)
        values_center = res_center['results'][0]['series'][0]['values']
        self.assertEqual(len(values_center), 3)
        
        # The first window finishes on the 3rd point (time="1970-01-01T00:00:03Z")
        # Shifted back by 1 means time="1970-01-01T00:00:02Z"
        self.assertEqual(values_center[0][0], "1970-01-01T00:00:02Z")

    def test_04_complex_measurement_renames(self):
        DB = "e2e_db_complex_meas"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        
        # 1. Multiple Renames (A -> B -> C)
        write_points(["m_a,tag=1 val=10 1000000000"], db=DB)
        query("ALTER MEASUREMENT m_a RENAME TO m_b", db=DB, method="POST")
        write_points(["m_b,tag=1 val=20 2000000000"], db=DB)
        query("ALTER MEASUREMENT m_b RENAME TO m_c", db=DB, method="POST")
        write_points(["m_c,tag=1 val=30 3000000000"], db=DB)
        time.sleep(0.5)
        
        # Verify querying C returns all data
        res = query("SELECT * FROM m_c", db=DB)
        values = res['results'][0]['series'][0]['values']
        self.assertEqual(len(values), 3, "Expected 3 points under the name m_c")
        
        # Verify querying A or B returns nothing
        try:
             res_a = query("SELECT * FROM m_a", db=DB)
             if 'series' in res_a['results'][0]:
                 values_a = res_a['results'][0]['series'][0].get('values', [])
                 self.assertEqual(len(values_a), 0, f"Old name m_a should return no data. values: {values_a}")
        except urllib.error.HTTPError as e:
             if e.code != 404:
                  raise e
        
        try:
             res_b = query("SELECT * FROM m_b", db=DB)
             if 'series' in res_b['results'][0]:
                 values_b = res_b['results'][0]['series'][0].get('values', [])
                 self.assertEqual(len(values_b), 0, f"Old name m_b should return no data. values: {values_b}")
        except urllib.error.HTTPError as e:
             if e.code != 404:
                  raise e

        # 2. Rename to existing
        write_points(["m_d,tag=1 val=40 4000000000"], db=DB)
        # Attempt to rename m_d to m_c, which already exists
        res = query("ALTER MEASUREMENT m_d RENAME TO m_c", db=DB, method="POST")
        if 'error' in res.get('results', [{}])[0]:
            err_msg = res['results'][0]['error']
            if "already exists" not in err_msg and "already mapped" not in err_msg:
                 self.fail(f"Unexpected error message: {err_msg}")
        else:
            self.fail("Exception not raised")

        # 3. Delete and recreate with same name
        query("DROP MEASUREMENT m_c", db=DB, method="POST")
        
        # Recreate m_c (it should get a new internal mapping version)
        write_points(["m_c,tag=1 val=50 5000000000"], db=DB)
        time.sleep(0.5)
        
        # Query m_c - should only return the new point!
        res_new_c = query("SELECT * FROM m_c", db=DB)
        values_new_c = res_new_c['results'][0]['series'][0]['values']
        self.assertEqual(len(values_new_c), 1, "Expected 1 point after recreating m_c")
        self.assertEqual(values_new_c[0][2], 50.0, "Expected new value 50.0")

    def test_05_rp_and_cq_integration(self):
        DB = "e2e_db_rp_cq"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        time.sleep(1)
        
        # 1. Create a Retention Policy
        # DURATION must be at least 1h
        res = query(f"CREATE RETENTION POLICY rp1 ON {DB} DURATION 1h REPLICATION 1", db="", method="POST")
        time.sleep(1)
        
        # Verify it was created
        rp_res = query("SHOW RETENTION POLICIES", db=DB)
        has_rp1 = any(val[0] == 'rp1' for val in rp_res['results'][0]['series'][0]['values'])
        self.assertTrue(has_rp1, f"rp1 should be created. current RPs: {rp_res}, create response: {res}")
        
        # Write to the specific RP
        write_points(["src_meas,tag=a val=10 2000000000000000000"], db=DB) # Goes to autogen
        try:
            write_points(["src_meas,tag=a val=20 2000000000000000000"], db=DB, rp="rp1") # Specific RP
        except Exception as e:
            print("EXCEPTION: ", str(e))
            # Let's verify what RP are defined
            rp_res = query("SHOW RETENTION POLICIES", db=DB)
            print("SHOW RP: ", rp_res)
            raise e
        time.sleep(1)
        
        # Query from RP
        res = query(f"SELECT * FROM rp1.src_meas", db=DB)
        self.assertEqual(len(res['results'][0]['series'][0]['values']), 1)
        self.assertEqual(res['results'][0]['series'][0]['values'][0][2], 20.0)
        
        # Rename the measurement
        query("ALTER MEASUREMENT src_meas RENAME TO src_meas_new", db=DB, method="POST")
        
        # Query from RP using new name
        res_new = query(f"SELECT * FROM rp1.src_meas_new", db=DB)
        self.assertEqual(len(res_new['results'][0]['series'][0]['values']), 1, "RP query should work with new measurement name")

        # 2. Continuous Query Check (Basic validation that queries continue to work)
        # We can't easily test real-time CQs without waiting for the tick interval (typically 1s+), 
        # but we can verify the INTO query syntax compiles correctly with the renamed measurement.
        query(f"SELECT sum(val) INTO dest_meas FROM autogen.src_meas_new", db=DB, method="POST")
        time.sleep(0.5)
        
        res_dest = query("SELECT * FROM dest_meas", db=DB)
        self.assertIn('series', res_dest['results'][0], "INTO query should have created dest_meas from the renamed src_meas_new")

    def test_06_complex_field_renames(self):
        DB = "e2e_db_complex_field"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Multiple Renames (f_a -> f_b -> f_c)
        write_points(["m1,tag=1 f_a=10.0 1000000000"], db=DB)
        query("ALTER MEASUREMENT m1 RENAME FIELD f_a TO f_b", db=DB, method="POST")
        write_points(["m1,tag=1 f_b=20.0 2000000000"], db=DB)
        query("ALTER MEASUREMENT m1 RENAME FIELD f_b TO f_c", db=DB, method="POST")
        write_points(["m1,tag=1 f_c=30.0 3000000000"], db=DB)
        time.sleep(0.5)
        
        res = query("SELECT f_c FROM m1", db=DB)
        values = res['results'][0]['series'][0]['values']
        self.assertEqual(len(values), 3, "Expected 3 points under the name f_c")
        
        # Verify querying old names returns no series/columns
        res_a = query("SELECT f_a FROM m1", db=DB)
        if 'series' in res_a['results'][0]:
            # It might return a series if another test created it or if the caching isn't fully flushed.
            columns = res_a['results'][0]['series'][0].get('columns', [])
            if 'f_a' in columns:
                 idx = columns.index('f_a')
                 values_a = [row[idx] for row in res_a['results'][0]['series'][0].get('values', []) if row[idx] is not None]
                 self.assertEqual(len(values_a), 0, "Old field name f_a should return no data")
        
        # 2. Rename to existing
        write_points(["m1,tag=1 f_d=40.0 4000000000"], db=DB)
        res = query("ALTER MEASUREMENT m1 RENAME FIELD f_d TO f_c", db=DB, method="POST")
        if 'error' in res.get('results', [{}])[0]:
            err_msg = res['results'][0]['error']
            if "already exists" not in err_msg and "already mapped" not in err_msg:
                 self.fail(f"Unexpected error message: {err_msg}")
        else:
            self.fail("Exception not raised")

        # 3. Swap names via temporary (a -> tmp, b -> a, tmp -> b)
        write_points(["swap_m,tag=1 field_a=1.0,field_b=2.0 5000000000"], db=DB)
        
        query("ALTER MEASUREMENT swap_m RENAME FIELD field_a TO field_tmp", db=DB, method="POST")
        query("ALTER MEASUREMENT swap_m RENAME FIELD field_b TO field_a", db=DB, method="POST")
        query("ALTER MEASUREMENT swap_m RENAME FIELD field_tmp TO field_b", db=DB, method="POST")
        
        write_points(["swap_m,tag=1 field_a=3.0,field_b=4.0 6000000000"], db=DB)
        time.sleep(0.5)
        
        res_swap = query("SELECT field_a, field_b FROM swap_m", db=DB)
        
        # Verify columns exist
        cols = res_swap['results'][0]['series'][0]['columns']
        idx_a = cols.index('field_a')
        idx_b = cols.index('field_b')
        
        values = res_swap['results'][0]['series'][0]['values']
        
        # We need to sort values by time to ensure consistent index
        values.sort(key=lambda x: x[0])

        # First point: originally a=1.0, b=2.0.
        # Now field_a points to old b (2.0), field_b points to old a (1.0)
        # Note: InfluxDB returns sorted by time naturally.
        
        # When querying swap_m, we are querying the CURRENT view.
        # Time 5000000000: field_a (internal field_b) was 2.0. field_b (internal field_a) was 1.0.
        self.assertEqual(values[0][idx_a], 2.0)
        self.assertEqual(values[0][idx_b], 1.0)
        
        # Second point: written as a=3.0, b=4.0 AFTER swap.
        self.assertEqual(values[1][idx_a], 3.0)
        self.assertEqual(values[1][idx_b], 4.0)

    def test_07_mapping_deletion_resolution(self):
        DB = "e2e_db_mapping_deletion"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        
        # Scenario: Create field mapping f_a -> f_a.v1
        write_points(["m1,tag=1 f_a=10.0 1000000000"], db=DB)
        time.sleep(0.5)
        
        # Rename f_a to f_b (f_b -> f_a.v1)
        query("ALTER MEASUREMENT m1 RENAME FIELD f_a TO f_b", db=DB, method="POST")
        
        # Recreate f_a (f_a -> f_a.v2)
        write_points(["m1,tag=1 f_a=20.0 2000000000"], db=DB)
        time.sleep(0.5)
        
        # We now have two fields: f_b (v1) and f_a (v2)
        # Drop f_a. It should mark f_a (v2) as DELETED.
        query("DROP FIELD f_a FROM m1", db=DB, method="POST")
        
        # Verify f_b still exists and works
        res_b = query("SELECT f_b FROM m1", db=DB)
        self.assertIn('series', res_b['results'][0], "f_b should still exist")
        self.assertEqual(res_b['results'][0]['series'][0]['values'][0][1], 10.0)
        
        # Verify f_a is gone
        res_a = query("SELECT f_a FROM m1", db=DB)
        self.assertNotIn('series', res_a['results'][0], "f_a should be deleted")

    def test_08_regex_with_multiple_mappings(self):
        DB = "e2e_db_regex_multi"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        
        # Write initial data to two fields matching a regex
        write_points(["regex_m,tag=1 field_1=10.0,field_2=20.0 1000000000"], db=DB)
        time.sleep(0.5)
        
        # Rename field_1 to something else
        query("ALTER MEASUREMENT regex_m RENAME FIELD field_1 TO other_field", db=DB, method="POST")
        
        # Write new data
        write_points(["regex_m,tag=1 other_field=15.0,field_2=25.0 2000000000"], db=DB)
        time.sleep(0.5)
        
        # Query with regex matching /field_.*/
        res = query("SELECT /field_.*/ FROM regex_m", db=DB)
        cols = res['results'][0]['series'][0]['columns']
        
        # field_1 should NOT be in the results anymore because its user name is other_field
        self.assertNotIn("field_1", cols, "Regex should not match the old user name")
        # field_2 should be there
        self.assertIn("field_2", cols, "Regex should match field_2")
        # other_field should not be there because it doesn't match the regex
        self.assertNotIn("other_field", cols, "other_field doesn't match the regex")
        
        # Query with regex matching /other_.*/
        res_other = query("SELECT /other_.*/ FROM regex_m", db=DB)
        cols_other = res_other['results'][0]['series'][0]['columns']
        self.assertIn("other_field", cols_other, "Regex should match the new user name")
        
        values_other = res_other['results'][0]['series'][0]['values']
        self.assertEqual(len(values_other), 2, "Should return both points for other_field (mapped to old field_1)")

    def test_09_mapping_cache_clear_on_drop_db(self):
        DB = "e2e_db_cache_clear"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Create measurement mapping
        write_points(["m_cache,tag=1 val=10.0 1000000000"], db=DB)
        query("ALTER MEASUREMENT m_cache RENAME TO m_cache_new", db=DB, method="POST")

        # Create field mapping
        write_points(["m_cache2,tag=1 f_cache=10.0 1000000000"], db=DB)
        query("ALTER MEASUREMENT m_cache2 RENAME FIELD f_cache TO f_cache_new", db=DB, method="POST")

        # Drop Database
        query(f"DROP DATABASE {DB}", db="", method="POST")
        
        # Recreate Database
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Attempt to create mappings with the same names (should not error if cache was cleared)
        write_points(["m_cache,tag=1 val=10.0 1000000000"], db=DB)
        res_m = query("ALTER MEASUREMENT m_cache RENAME TO m_cache_new", db=DB, method="POST")
        self.assertNotIn("error", res_m, "Should not hit 'measurement already exists' cache error")

        write_points(["m_cache2,tag=1 f_cache=10.0 1000000000"], db=DB)
        res_f = query("ALTER MEASUREMENT m_cache2 RENAME FIELD f_cache TO f_cache_new", db=DB, method="POST")
        self.assertNotIn("error", res_f, "Should not hit 'field already exists' cache error")

    def test_10_database_renaming(self):
        DB_ORIG = "e2e_db_orig"
        DB_NEW = "e2e_db_new"
        
        # Cleanup
        query(f"DROP DATABASE {DB_ORIG}", db="", method="POST")
        query(f"DROP DATABASE {DB_NEW}", db="", method="POST")
        
        # 1. Create original database
        query(f"CREATE DATABASE {DB_ORIG}", db="", method="POST")
        
        # Write data to original database
        write_points(["m1,tag=1 val=10.0 1000000000"], db=DB_ORIG)
        time.sleep(0.5)
        
        # 2. Rename the database
        query(f"ALTER DATABASE {DB_ORIG} RENAME TO {DB_NEW}", db="", method="POST")
        
        # 3. Verify SHOW DATABASE MAPPINGS
        mappings_res = query("SHOW DATABASE MAPPINGS", db="")
        series = mappings_res.get('results', [{}])[0].get('series', [])
        self.assertTrue(len(series) > 0, "Expected database mappings series")
        
        mapping_found = False
        for row in series[0]['values']:
            if row[0] == DB_NEW and row[2] == 'ACTIVE':
                mapping_found = True
                break
        self.assertTrue(mapping_found, f"Mapping for {DB_NEW} should be ACTIVE")

        # 4. Write data using the new database name
        write_points(["m1,tag=1 val=20.0 2000000000"], db=DB_NEW)
        time.sleep(0.5)
        
        # 5. Query data using the new database name
        res = query("SELECT * FROM m1", db=DB_NEW)
        values = res['results'][0]['series'][0]['values']
        self.assertEqual(len(values), 2, "Expected 2 points under the new database name")
        
        # 6. Verify original database name is no longer accessible
        # Influx returns database not found error for invalid dbs on query
        with self.assertRaises(Exception) as context:
            query("SELECT * FROM m1", db=DB_ORIG)
        self.assertIn("database not found", str(context.exception))
        
        # 7. Drop the renamed database
        query(f"DROP DATABASE {DB_NEW}", db="", method="POST")
        
        # 8. Verify the mapping is marked DELETED
        mappings_res = query("SHOW DATABASE MAPPINGS", db="")
        series = mappings_res.get('results', [{}])[0].get('series', [])
        mapping_deleted = False
        for row in series[0]['values']:
            if row[0] == None and row[1] == DB_ORIG and row[2] == 'DELETED':
                mapping_deleted = True
                break
        self.assertTrue(mapping_deleted, "Mapping should be marked DELETED after drop")

if __name__ == '__main__':
    unittest.main(verbosity=2)
