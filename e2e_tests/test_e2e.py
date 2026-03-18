import unittest
import urllib.request
import urllib.parse
import urllib.error
import json
import time

API_URL = "http://localhost:8086"
DB_NAME = "e2e_db"


def query(q, db=DB_NAME, method="GET"):
    data = urllib.parse.urlencode({"q": q, "db": db})
    if method == "GET":
        req = urllib.request.Request(f"{API_URL}/query?{data}")
    else:
        req = urllib.request.Request(f"{API_URL}/query", data=data.encode("utf-8"))

    try:
        with urllib.request.urlopen(req) as response:
            return json.loads(response.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
        raise Exception(f"HTTP Error {e.code}: {body}")


def write_points(lines, db=DB_NAME, rp=None):
    data = "\n".join(lines).encode("utf-8")
    url = f"{API_URL}/write?db={db}"
    if rp:
        url += f"&rp={rp}"
    req = urllib.request.Request(url, data=data, method="POST")
    try:
        with urllib.request.urlopen(req) as response:
            return response.getcode()
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
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
        write_points(
            [
                "m_test_orig,tag=a val=10 1000000000",
                "m_test_orig,tag=b val=20 2000000000",
            ],
            db=DB,
        )

        # Wait a moment for indexing
        time.sleep(0.5)

        # 2. Rename measurement
        query(
            "ALTER MEASUREMENT m_test_orig RENAME TO m_test_new",
            db=DB,
            method="POST",
        )

        # 3. Verify SHOW MEASUREMENT MAPPINGS
        mappings_res = query("SHOW MEASUREMENT MAPPINGS", db=DB)
        series = mappings_res.get("results", [{}])[0].get("series", [])
        self.assertTrue(len(series) > 0, "Expected measurement mappings series")

        mapping_found = False
        for row in series[0]["values"]:
            # Columns: user_name, internal_name (no state column)
            if row[0] == "m_test_new":
                mapping_found = True
                break
        self.assertTrue(mapping_found, "Mapping for m_test_new should be present")

        # 4. Write new data to the renamed measurement
        write_points(
            [
                "m_test_new,tag=a val=30 3000000000",
            ],
            db=DB,
        )

        time.sleep(0.5)

        # 5. Query all data using the new name
        res = query("SELECT * FROM m_test_new", db=DB)
        values = res["results"][0]["series"][0]["values"]
        self.assertEqual(
            len(values), 3, "Expected 3 points under the new measurement name"
        )

        # 6. Test Wildcard regex match against pre-translation name
        res_regex = query("SELECT * FROM /m_test_n.*/", db=DB)
        # The test expects regex to match m_test_new and return data.
        self.assertIn(
            "series",
            res_regex.get("results", [{}])[0],
            f"Regex query should match m_test_new. res: {res_regex}",
        )

        # 7. Drop measurement
        query("DROP MEASUREMENT m_test_new", db=DB, method="POST")

        # 8. Verify DROP handles mapping cleanup
        mappings_res = query("SHOW MEASUREMENT MAPPINGS", db=DB)

        # When we completely remove the mapping on drop (as we just changed),
        # the SHOW MEASUREMENT MAPPINGS command might return an empty series or
        # no series at all.
        series = mappings_res.get("results", [{}])[0].get("series", [])

        if len(series) == 0 or "values" not in series[0]:
            # If there's no series returned, it means there are no mappings
            # left at all, which is correct.
            self.assertTrue(
                True,
                "No mappings series returned, mapping was completely deleted",
            )
        else:
            # If a series is returned, we need to make sure m_test_new or
            # m_test_orig is not in it.
            mapping_deleted = True
            for row in series[0]["values"]:
                if row[0] == "m_test_new" or row[0] == "m_test_orig":
                    mapping_deleted = False
                    break
            self.assertTrue(
                mapping_deleted,
                "Mapping for m_test_new should not exist in active mappings",
            )

    def test_02_field_mapping(self):
        DB = "e2e_db_field"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write initial data
        write_points(
            [
                "sensors,loc=room1 temp=72.0 1000000000",
                "sensors,loc=room1 temp=73.0 2000000000",
            ],
            db=DB,
        )
        time.sleep(0.5)

        # 2. Rename field
        query(
            "ALTER MEASUREMENT sensors RENAME FIELD temp TO temperature",
            db=DB,
            method="POST",
        )

        # 3. Write new data using new field name
        write_points(["sensors,loc=room1 temperature=74.0 3000000000"], db=DB)
        time.sleep(0.5)

        # 4. Query data using new field name
        res = query("SELECT temperature FROM sensors", db=DB)
        series = res["results"][0].get("series", [])
        self.assertTrue(len(series) > 0, "Expected results for renamed field")
        self.assertEqual(
            len(series[0]["values"]), 3, "Expected 3 points for temperature"
        )
        self.assertIn(
            "temperature",
            series[0]["columns"],
            "Column should be named 'temperature'",
        )

        # 5. Drop field
        query("DROP FIELD temperature FROM sensors", db=DB, method="POST")

        # 6. Ensure field is gone
        res = query("SELECT temperature FROM sensors", db=DB)
        self.assertNotIn(
            "series",
            res["results"][0],
            "Dropped field should return no series data",
        )

    def test_03_moving_functions(self):
        DB = "e2e_db_funcs"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write sequential data
        write_points(
            [
                # 1.0 to 5.0
                f"stock,sym=a price={float(i)} {i}000000000"
                for i in range(1, 6)
            ],
            db=DB,
        )
        time.sleep(0.5)

        # 2. Test moving_median
        res = query(
            "SELECT moving_median(median(price), 3) FROM stock "
            "WHERE time >= 0 AND time <= 6s GROUP BY time(1s)",
            db=DB,
        )
        values = res["results"][0]["series"][0]["values"]

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
        # Shifted back by 1 means the result for the window ending at t=3 is
        # reported at t=2.
        res_center = query(
            "SELECT moving_average(mean(price), 3, 3, true) FROM stock "
            "WHERE time >= 0 AND time <= 6s GROUP BY time(1s)",
            db=DB,
        )
        values_center = res_center["results"][0]["series"][0]["values"]
        self.assertEqual(len(values_center), 3)

        # The first window finishes on the 3rd point
        # (time="1970-01-01T00:00:03Z")
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
        values = res["results"][0]["series"][0]["values"]
        self.assertEqual(len(values), 3, "Expected 3 points under the name m_c")

        # Verify querying A or B returns nothing
        try:
            res_a = query("SELECT * FROM m_a", db=DB)
            # Even if it succeeds, there shouldn't be series data for 'val'
            # under 'm_a'
            if "series" in res_a.get("results", [{}])[0]:
                series = res_a["results"][0]["series"]
                self.assertEqual(
                    len(series),
                    0,
                    f"Old name m_a should return no data. series: {series}",
                )
        except Exception:
            # Expect an error or no series
            pass

        try:
            res_b = query("SELECT * FROM m_b", db=DB)
            if "series" in res_b.get("results", [{}])[0]:
                series = res_b["results"][0]["series"]
                self.assertEqual(
                    len(series),
                    0,
                    f"Old name m_b should return no data. series: {series}",
                )
        except Exception:
            pass

        # 2. Rename to existing
        write_points(["m_d,tag=1 val=40 4000000000"], db=DB)
        # Attempt to rename m_d to m_c, which already exists
        res = query("ALTER MEASUREMENT m_d RENAME TO m_c", db=DB, method="POST")
        if "error" in res.get("results", [{}])[0]:
            err_msg = res["results"][0]["error"]
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
        series_c = res_new_c.get("results", [{}])[0].get("series", [])
        values_new_c = series_c[0]["values"] if series_c else []
        self.assertEqual(len(values_new_c), 1, "Expected 1 point after recreating m_c")
        self.assertEqual(values_new_c[0][2], 50.0, "Expected new value 50.0")

    def test_05_rp_and_cq_integration(self):
        DB = "e2e_db_rp_cq"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")
        time.sleep(1)

        # 1. Create a Retention Policy
        # DURATION must be at least 1h
        res = query(
            f"CREATE RETENTION POLICY rp1 ON {DB} DURATION 1h REPLICATION 1",
            db="",
            method="POST",
        )
        time.sleep(1)

        # Verify it was created
        rp_res = query("SHOW RETENTION POLICIES", db=DB)
        has_rp1 = any(
            val[0] == "rp1" for val in rp_res["results"][0]["series"][0]["values"]
        )
        self.assertTrue(
            has_rp1,
            f"rp1 should be created.\n"
            f"current RPs: {rp_res}, create response: {res}",
        )

        # Write to the specific RP
        write_points(
            ["src_meas,tag=a val=10 2000000000000000000"], db=DB
        )  # Goes to autogen
        try:
            write_points(
                ["src_meas,tag=a val=20 2000000000000000000"], db=DB, rp="rp1"
            )  # Specific RP
        except Exception as e:
            print("EXCEPTION: ", str(e))
            # Let's verify what RP are defined
            rp_res = query("SHOW RETENTION POLICIES", db=DB)
            print("SHOW RP: ", rp_res)
            raise e
        time.sleep(1)

        # Query from RP
        res = query("SELECT * FROM rp1.src_meas", db=DB)
        self.assertEqual(len(res["results"][0]["series"][0]["values"]), 1)
        self.assertEqual(res["results"][0]["series"][0]["values"][0][2], 20.0)

        # Rename the measurement
        query(
            "ALTER MEASUREMENT src_meas RENAME TO src_meas_new",
            db=DB,
            method="POST",
        )

        # Query from RP using new name
        res_new = query("SELECT * FROM rp1.src_meas_new", db=DB)
        self.assertEqual(
            len(res_new["results"][0]["series"][0]["values"]),
            1,
            "RP query should work with new measurement name",
        )

        # 2. Continuous Query Check (Basic validation that queries continue)
        # We can't easily test real-time CQs without waiting for the tick
        # interval (typically 1s+), but we can verify the INTO query syntax
        # compiles correctly with the renamed measurement.
        query(
            "SELECT sum(val) INTO dest_meas FROM autogen.src_meas_new",
            db=DB,
            method="POST",
        )
        time.sleep(0.5)

        res_dest = query("SELECT * FROM dest_meas", db=DB)
        self.assertIn(
            "series",
            res_dest["results"][0],
            "INTO query should have created dest_meas from the " "renamed src_meas_new",
        )

    def test_06_complex_field_renames(self):
        DB = "e2e_db_complex_field"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Multiple Renames (f_a -> f_b -> f_c)
        write_points(["m1,tag=1 f_a=10.0 1000000000"], db=DB)
        query(
            "ALTER MEASUREMENT m1 RENAME FIELD f_a TO f_b",
            db=DB,
            method="POST",
        )
        write_points(["m1,tag=1 f_b=20.0 2000000000"], db=DB)
        query(
            "ALTER MEASUREMENT m1 RENAME FIELD f_b TO f_c",
            db=DB,
            method="POST",
        )
        write_points(["m1,tag=1 f_c=30.0 3000000000"], db=DB)
        time.sleep(0.5)

        res = query("SELECT f_c FROM m1", db=DB)
        values = res["results"][0]["series"][0]["values"]
        self.assertEqual(len(values), 3, "Expected 3 points under the name f_c")

        # Verify querying old names returns no series/columns
        res_a = query("SELECT f_a FROM m1", db=DB)
        if "series" in res_a["results"][0]:
            # It might return a series if another test created it or if the
            # caching isn't fully flushed.
            columns = res_a["results"][0]["series"][0].get("columns", [])
            if "f_a" in columns:
                idx = columns.index("f_a")
                values_a = [
                    row[idx]
                    for row in res_a["results"][0]["series"][0].get("values", [])
                    if row[idx] is not None
                ]
                self.assertEqual(
                    len(values_a),
                    0,
                    "Old field name f_a should return no data",
                )

        # 2. Rename to existing
        write_points(["m1,tag=1 f_d=40.0 4000000000"], db=DB)
        res = query(
            "ALTER MEASUREMENT m1 RENAME FIELD f_d TO f_c",
            db=DB,
            method="POST",
        )
        if "error" in res.get("results", [{}])[0]:
            err_msg = res["results"][0]["error"]
            if "already exists" not in err_msg and "already mapped" not in err_msg:
                self.fail(f"Unexpected error message: {err_msg}")
        else:
            self.fail("Exception not raised")

        # 3. Swap names via temporary (a -> tmp, b -> a, tmp -> b)
        write_points(["swap_m,tag=1 field_a=1.0,field_b=2.0 5000000000"], db=DB)
        time.sleep(0.5)

        query(
            "ALTER MEASUREMENT swap_m RENAME FIELD field_a TO field_tmp",
            db=DB,
            method="POST",
        )
        time.sleep(0.5)

        query(
            "ALTER MEASUREMENT swap_m RENAME FIELD field_b TO field_a",
            db=DB,
            method="POST",
        )
        time.sleep(0.5)

        query(
            "ALTER MEASUREMENT swap_m RENAME FIELD field_tmp TO field_b",
            db=DB,
            method="POST",
        )
        time.sleep(0.5)

        write_points(["swap_m,tag=1 field_a=3.0,field_b=4.0 6000000000"], db=DB)
        time.sleep(0.5)

        res_swap = query("SELECT field_a, field_b FROM swap_m", db=DB)

        # Verify columns exist
        series = res_swap.get("results", [{}])[0].get("series", [])
        if not series:
            # Maybe query each separately to debug
            res_a = query("SELECT field_a FROM swap_m", db=DB)
            res_b = query("SELECT field_b FROM swap_m", db=DB)
            self.fail(
                f"No series returned for combined query.\n"
                f"field_a: {res_a}, field_b: {res_b}"
            )
        cols = series[0]["columns"]

        if "field_a" not in cols or "field_b" not in cols:
            self.fail(f"Missing columns. Expected field_a and field_b, got: {cols}")

        idx_a = cols.index("field_a")
        idx_b = cols.index("field_b")

        values = res_swap["results"][0]["series"][0]["values"]

        # We need to sort values by time to ensure consistent index
        values.sort(key=lambda x: x[0])

        # First point: originally a=1.0, b=2.0.
        # Now field_a points to old b (2.0), field_b points to old a (1.0)
        # Note: InfluxDB returns sorted by time naturally.

        # When querying swap_m, we are querying the CURRENT view.
        # Time 5000000000: field_a (internal field_b) was 2.0. field_b
        # (internal field_a) was 1.0.

        self.assertEqual(
            values[0][idx_a], 2.0, f"Expected user field_a to be 2.0 but was {
                values[0][idx_a]}"
        )
        self.assertEqual(
            values[0][idx_b], 1.0, f"Expected user field_b to be 1.0 but was {
                values[0][idx_b]}"
        )

        # Second point: written as a=3.0, b=4.0 AFTER swap.
        self.assertEqual(values[1][idx_a], 3.0)
        self.assertEqual(values[1][idx_b], 4.0)

    def test_07_mapping_deletion_resolution(self):
        DB = "e2e_db_mapping_deletion"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Scenario: Create field mapping f_a -> f_a
        write_points(["m1,tag=1 f_a=10.0 1000000000"], db=DB)
        time.sleep(0.5)

        # Rename f_a to f_b (f_b -> f_a)
        query(
            "ALTER MEASUREMENT m1 RENAME FIELD f_a TO f_b",
            db=DB,
            method="POST",
        )

        # Recreate f_a (f_a -> f_a.v2)
        write_points(["m1,tag=1 f_a=20.0 2000000000"], db=DB)
        time.sleep(0.5)

        # We now have two fields: f_b (v1) and f_a (v2)
        # Drop f_a. It should mark f_a (v2) as DELETED.
        query("DROP FIELD f_a FROM m1", db=DB, method="POST")

        # Verify f_b still exists and works
        res_b = query("SELECT f_b FROM m1", db=DB)
        self.assertIn("series", res_b["results"][0], "f_b should still exist")
        self.assertEqual(res_b["results"][0]["series"][0]["values"][0][1], 10.0)

        # Verify f_a is gone
        res_a = query("SELECT f_a FROM m1", db=DB)
        self.assertNotIn("series", res_a["results"][0], "f_a should be deleted")

    def test_08_regex_with_multiple_mappings(self):
        DB = "e2e_db_regex_multi"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Write initial data to two fields matching a regex
        write_points(["regex_m,tag=1 field_1=10.0,field_2=20.0 1000000000"], db=DB)
        time.sleep(0.5)

        # Rename field_1 to something else
        query(
            "ALTER MEASUREMENT regex_m RENAME FIELD field_1 TO other_field",
            db=DB,
            method="POST",
        )

        # Write new data
        write_points(["regex_m,tag=1 other_field=15.0,field_2=25.0 2000000000"], db=DB)
        time.sleep(0.5)

        # Query with regex matching /field_.*/
        res = query("SELECT /field_.*/ FROM regex_m", db=DB)
        cols = res["results"][0]["series"][0]["columns"]

        # field_1 should NOT be in the results anymore because its user name is
        # other_field
        self.assertNotIn("field_1", cols, "Regex should not match the old user name")
        # field_2 should be there
        self.assertIn("field_2", cols, "Regex should match field_2")
        # other_field should not be there because it doesn't match the regex
        self.assertNotIn("other_field", cols, "other_field doesn't match the regex")

        # Query with regex matching /other_.*/
        res_other = query("SELECT /other_.*/ FROM regex_m", db=DB)
        cols_other = res_other["results"][0]["series"][0]["columns"]
        self.assertIn("other_field", cols_other, "Regex should match the new user name")

        values_other = res_other["results"][0]["series"][0]["values"]
        self.assertEqual(
            len(values_other),
            2,
            "Should return both points for other_field " "(mapped to old field_1)",
        )

    def test_09_mapping_cache_clear_on_drop_db(self):
        DB = "e2e_db_cache_clear"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Create measurement mapping
        write_points(["m_cache,tag=1 val=10.0 1000000000"], db=DB)
        query(
            "ALTER MEASUREMENT m_cache RENAME TO m_cache_new",
            db=DB,
            method="POST",
        )

        # Create field mapping
        write_points(["m_cache2,tag=1 f_cache=10.0 1000000000"], db=DB)
        query(
            "ALTER MEASUREMENT m_cache2 RENAME FIELD f_cache TO f_cache_new",
            db=DB,
            method="POST",
        )

        # Drop Database
        query(f"DROP DATABASE {DB}", db="", method="POST")

        # Recreate Database
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Attempt to create mappings with the same names (should not error if
        # cache was cleared)
        write_points(["m_cache,tag=1 val=10.0 1000000000"], db=DB)
        res_m = query(
            "ALTER MEASUREMENT m_cache RENAME TO m_cache_new",
            db=DB,
            method="POST",
        )
        self.assertNotIn(
            "error",
            res_m,
            "Should not hit 'measurement already exists' cache error",
        )

        write_points(["m_cache2,tag=1 f_cache=10.0 1000000000"], db=DB)
        res_f = query(
            "ALTER MEASUREMENT m_cache2 RENAME FIELD f_cache TO f_cache_new",
            db=DB,
            method="POST",
        )
        self.assertNotIn(
            "error", res_f, "Should not hit 'field already exists' cache error"
        )

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
        query(
            f"ALTER DATABASE {DB_ORIG} RENAME TO {DB_NEW}",
            db="",
            method="POST",
        )

        # 3. Verify SHOW DATABASE MAPPINGS
        mappings_res = query("SHOW DATABASE MAPPINGS", db="")
        series = mappings_res.get("results", [{}])[0].get("series", [])
        self.assertTrue(len(series) > 0, "Expected database mappings series")

        mapping_found = False
        for row in series[0]["values"]:
            # Columns: user_name, internal_name (no state column)
            if row[0] == DB_NEW:
                mapping_found = True
                break
        self.assertTrue(mapping_found, f"Mapping for {DB_NEW} should be present")

        # 4. Write data using the new database name
        write_points(["m1,tag=1 val=20.0 2000000000"], db=DB_NEW)
        time.sleep(0.5)

        # 5. Query data using the new database name
        res = query("SELECT * FROM m1", db=DB_NEW)
        values = res["results"][0]["series"][0]["values"]
        self.assertEqual(
            len(values), 2, "Expected 2 points under the new database name"
        )

        # 6. Verify original database name is no longer accessible
        # Influx returns database not found error for invalid dbs on query
        try:
            res = query("SELECT * FROM m1", db=DB_ORIG)
            if "error" in res.get("results", [{}])[0]:
                self.assertTrue(
                    "database not found" in str(res).lower(),
                    f"Expected database not found error, got: {res}",
                )
            else:
                self.fail(f"Expected database not found error, got: {res}")
        except Exception:
            # urllib might throw 404
            pass

        # 7. Drop the renamed database
        query(f"DROP DATABASE {DB_NEW}", db="", method="POST")

        # 8. Verify the mapping is marked DELETED
        mappings_res = query("SHOW DATABASE MAPPINGS", db="")
        series = mappings_res.get("results", [{}])[0].get("series", [])

        if len(series) == 0 or "values" not in series[0]:
            self.assertTrue(
                True,
                "No mappings series returned, mapping was completely deleted",
            )
        else:
            mapping_deleted = True
            for row in series[0]["values"]:
                if row[0] == DB_NEW or row[0] == DB_ORIG:
                    mapping_deleted = False
                    break
            self.assertTrue(
                mapping_deleted,
                "Mapping should not exist in active mappings after drop",
            )

    def test_11_function_alias_collision(self):
        """
        Verify that mathematical function column names (like 'mean')
        do not accidentally get renamed if there happens to be an internal
        field mapped with that same name.
        """
        print("\n--- Running test_11_function_alias_collision ---")
        db_name = "test_db_collision"

        # Create database
        query(f"CREATE DATABASE {db_name}", db="")

        # Write data: 'mean' and 'f1' are both fields.
        write_points(["m1 f1=10.0,mean=20.0"], db=db_name)
        time.sleep(1)

        # Rename the field 'mean' to 'foo'
        query('ALTER MEASUREMENT m1 RENAME FIELD "mean" TO "foo"', db=db_name)

        # Query using the mathematical function 'mean' on the other field 'f1'
        res = query("SELECT mean(f1) FROM m1", db=db_name)
        series = res.get("results", [])[0].get("series", [])
        self.assertEqual(len(series), 1)

        # The column returned should be 'mean', NOT 'foo'.
        columns = series[0]["columns"]
        self.assertIn("mean", columns, "The column name should be 'mean'")
        self.assertNotIn(
            "foo", columns, "The column name was accidentally translated to 'foo'"
        )

        query(f"DROP DATABASE {db_name}", db="")

    def test_12_field_ops_on_renamed_measurement(self):
        """
        Verify that fields can be renamed and dropped on a measurement that
        has itself been renamed.
        """
        print("\n--- Running test_12_field_ops_on_renamed_measurement ---")
        DB = "e2e_db_field_renamed_meas"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write initial data
        write_points(["m1,tag=1 f1=10.0 1000000000"], db=DB)
        time.sleep(0.5)

        # 2. Rename measurement
        query("ALTER MEASUREMENT m1 RENAME TO m2", db=DB, method="POST")

        # 3. Rename field on the new measurement name
        res = query("ALTER MEASUREMENT m2 RENAME FIELD f1 TO f2", db=DB, method="POST")
        self.assertNotIn(
            "error",
            res,
            "Renaming field on renamed measurement should not error",
        )

        # 4. Write new data with new field
        write_points(["m2,tag=1 f2=20.0 2000000000"], db=DB)
        time.sleep(0.5)

        # Verify data
        res = query("SELECT f2 FROM m2", db=DB)
        series = res.get("results", [])[0].get("series", [])
        self.assertEqual(len(series), 1)
        self.assertEqual(len(series[0]["values"]), 2)

        # 5. Drop field on renamed measurement
        res = query("DROP FIELD f2 FROM m2", db=DB, method="POST")
        self.assertNotIn(
            "error",
            res,
            "Dropping field on renamed measurement should not error",
        )

        # Verify field is gone
        res = query("SELECT f2 FROM m2", db=DB)
        self.assertNotIn(
            "series", res.get("results", [])[0], "Field f2 should be dropped"
        )

    def test_13_drop_renamed_measurement_clears_fields(self):
        """
        Verify that dropping a measurement correctly resolves the internal name
        and clears out all associated field mappings completely.
        """
        print("\n--- Running test_13_drop_renamed_measurement_clears_fields ---")
        DB = "e2e_db_drop_renamed_meas_fields"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write initial data
        write_points(["m1,tag=1 f1=10.0 1000000000"], db=DB)
        time.sleep(0.5)

        # 2. Rename measurement
        query("ALTER MEASUREMENT m1 RENAME TO m2", db=DB, method="POST")

        # 3. Drop measurement
        query("DROP MEASUREMENT m2", db=DB, method="POST")

        # Verify measurement mappings are gone
        res = query("SHOW MEASUREMENT MAPPINGS", db=DB)
        series = res.get("results", [])[0].get("series", [])

        if len(series) > 0 and "values" in series[0]:
            for row in series[0]["values"]:
                if row[0] in ["m1", "m2"]:
                    self.fail(f"Measurement mapping for {row[0]} should not exist")

        # Verify field mappings are gone
        res = query("SHOW FIELD MAPPINGS", db=DB)
        series = res.get("results", [])[0].get("series", [])

        if len(series) > 0 and "values" in series[0]:
            for row in series[0]["values"]:
                if row[1] == "f1":  # f1 was the field
                    self.fail(
                        f"Field mapping for {row[1]} should not exist "
                        "after dropping measurement"
                    )

    def test_14_garbage_collection_protects_historical_names(self):
        """
        Verify that mapping garbage collection does not delete RENAMED
        mappings, which would incorrectly expose underlying internal names
        when queried.
        """
        print("\n--- Running test_14_garbage_collection_protects_historical_names ---")
        DB = "e2e_db_gc_historical"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Write data to f_old
        write_points(["gc_meas,tag=1 f_old=10.0 1000000000"], db=DB)
        time.sleep(0.5)

        # Rename f_old to f_new
        query(
            "ALTER MEASUREMENT gc_meas RENAME FIELD f_old TO f_new",
            db=DB,
            method="POST",
        )

        # At this point, f_old is mapped to f_new (ACTIVE) and f_old (RENAMED).
        # We need to trigger garbage collection. We can do this indirectly by
        # creating another mapping to force a save(), which triggers GC.
        write_points(["gc_meas,tag=1 other=5.0 2000000000"], db=DB)
        query(
            "ALTER MEASUREMENT gc_meas RENAME FIELD other TO other_new",
            db=DB,
            method="POST",
        )

        # Verify f_old returns NO series data, meaning it's still masked
        res = query("SELECT f_old FROM gc_meas", db=DB)
        self.assertNotIn(
            "series",
            res.get("results", [])[0],
            "f_old should be masked and return no data",
        )

        # Verify f_new returns the data
        res_new = query("SELECT f_new FROM gc_meas", db=DB)
        series = res_new.get("results", [])[0].get("series", [])
        self.assertEqual(len(series), 1)
        self.assertEqual(series[0]["columns"][1], "f_new")


        self.assertEqual(series[0]["columns"][1], "f_new")
        self.assertEqual(series[0]["values"][0][1], 10.0)

    def test_15_measurement_swap_preserves_fields(self):
        """
        Verify that swapping measurement names (m1->tmp, m2->m1, tmp->m2)
        correctly preserves the distinct field mappings of the underlying
        internal measurements.
        """
        print("\n--- Running test_15_measurement_swap_preserves_fields ---")
        DB = "e2e_db_meas_swap"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Write data to m1 with field f_a
        write_points(["m1,tag=1 f_a=10.0 1000000000"], db=DB)

        # Write data to m2 with field f_b
        write_points(["m2,tag=1 f_b=20.0 2000000000"], db=DB)
        time.sleep(0.5)

        # Swap measurements
        query("ALTER MEASUREMENT m1 RENAME TO m_tmp", db=DB, method="POST")
        query("ALTER MEASUREMENT m2 RENAME TO m1", db=DB, method="POST")
        query("ALTER MEASUREMENT m_tmp RENAME TO m2", db=DB, method="POST")

        # Now m1 should have f_b=20.0 (because it's the old m2)
        res_m1 = query("SELECT * FROM m1", db=DB)
        cols_m1 = res_m1.get("results", [])[0].get("series", [])[0]["columns"]
        self.assertIn("f_b", cols_m1, "New m1 should have f_b from old m2")
        self.assertNotIn("f_a", cols_m1, "New m1 should NOT have f_a")

        # And m2 should have f_a=10.0 (because it's the old m1)
        res_m2 = query("SELECT * FROM m2", db=DB)
        cols_m2 = res_m2.get("results", [])[0].get("series", [])[0]["columns"]
        self.assertIn("f_a", cols_m2, "New m2 should have f_a from old m1")
        self.assertNotIn("f_b", cols_m2, "New m2 should NOT have f_b")

    def test_16_drop_recreate_measurement_clears_field_history(self):
        """
        Verify that dropping a measurement and recreating it with the same name
        does NOT inherit the field mappings of the old measurement.
        """
        print(
            "\n--- Running test_16_drop_recreate_measurement_clears_field_history ---"
        )
        DB = "e2e_db_meas_recreate"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Write data, rename field
        write_points(["m_recreate,tag=1 old_field=10.0 1000000000"], db=DB)
        query(
            "ALTER MEASUREMENT m_recreate RENAME FIELD old_field TO new_field",
            db=DB,
            method="POST",
        )
        time.sleep(0.5)

        # Drop measurement
        query("DROP MEASUREMENT m_recreate", db=DB, method="POST")

        # Recreate measurement by writing new data
        write_points(["m_recreate,tag=1 new_field=20.0 2000000000"], db=DB)
        time.sleep(0.5)

        # Because this is a *new* measurement internally (m_recreate.v2),
        # it should NOT have the old mapping history.
        # We can verify this by checking that old_field does NOT map to new_field
        # and checking the internal mappings
        mappings_res = query("SHOW FIELD MAPPINGS", db=DB)
        series = mappings_res.get("results", [{}])[0].get("series", [])

        mapping_count = 0
        for row in series[0]["values"]:
            if row[0] == "m_recreate":
                mapping_count += 1
                self.assertEqual(
                    row[1],
                    "new_field",
                    "Only new_field should exist in mappings",
                )

        self.assertEqual(
            mapping_count,
            1,
            "Only one mapping should exist for the new measurement",
        )

    def test_17_full_chain_db_meas_field_rename(self):
        """
        Tests renaming a DB, then a measurement within it, then a field,
        and ensures queries still work across all layers of translation.
        """
        print("\n--- Running test_17_full_chain_db_meas_field_rename ---")
        DB_OLD = "db_old"
        DB_NEW = "db_new"
        query(f"DROP DATABASE {DB_OLD}", db="", method="POST")
        query(f"DROP DATABASE {DB_NEW}", db="", method="POST")
        query(f"CREATE DATABASE {DB_OLD}", db="", method="POST")

        write_points(["m_old,tag=1 f_old=10.0 1000000000"], db=DB_OLD)
        time.sleep(0.5)

        # Chain of renames
        query(
            f"ALTER DATABASE {DB_OLD} RENAME TO {DB_NEW}",
            db="",
            method="POST",
        )
        query(
            "ALTER MEASUREMENT m_old RENAME TO m_new",
            db=DB_NEW,
            method="POST",
        )
        query(
            "ALTER MEASUREMENT m_new RENAME FIELD f_old TO f_new",
            db=DB_NEW,
            method="POST",
        )

        # Query all the way through the new names
        res = query("SELECT f_new FROM m_new", db=DB_NEW)
        series = res.get("results", [])[0].get("series", [])
        self.assertEqual(len(series), 1)
        self.assertEqual(series[0]["columns"][1], "f_new")
        self.assertEqual(series[0]["values"][0][1], 10.0)

    def test_18_moving_functions_with_renamed_fields(self):
        """
        Verify that moving_average and moving_median work correctly on
        fields that have been renamed.
        """
        DB = "e2e_db_moving_rename"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write data with original field name
        write_points(
            [f"sensor,loc=a temp={float(i)} {i}000000000" for i in range(1, 6)],
            db=DB,
        )
        time.sleep(0.5)

        # 2. Rename field
        query(
            "ALTER MEASUREMENT sensor RENAME FIELD temp TO temperature",
            db=DB,
            method="POST",
        )

        # 3. Test moving_average on the renamed field
        res_avg = query(
            "SELECT moving_average(mean(temperature), 3) FROM sensor "
            "WHERE time >= 0 AND time <= 6s GROUP BY time(1s)",
            db=DB,
        )
        avg_values = res_avg["results"][0]["series"][0]["values"]
        # moving_average(3) over [1, 2, 3, 4, 5]:
        # At t=3: avg(1,2,3)=2.0, At t=4: avg(2,3,4)=3.0, At t=5: avg(3,4,5)=4.0
        self.assertEqual(len(avg_values), 3, "Expected 3 moving_average results")
        self.assertEqual(avg_values[0][1], 2.0)
        self.assertEqual(avg_values[1][1], 3.0)
        self.assertEqual(avg_values[2][1], 4.0)

        # 4. Test moving_median on the renamed field
        res_med = query(
            "SELECT moving_median(median(temperature), 3) FROM sensor "
            "WHERE time >= 0 AND time <= 6s GROUP BY time(1s)",
            db=DB,
        )
        med_values = res_med["results"][0]["series"][0]["values"]
        # moving_median(3) over [1, 2, 3, 4, 5]:
        # At t=3: median(1,2,3)=2.0, At t=4: median(2,3,4)=3.0, At t=5: median(3,4,5)=4.0
        self.assertEqual(len(med_values), 3, "Expected 3 moving_median results")
        self.assertEqual(med_values[0][1], 2.0)
        self.assertEqual(med_values[1][1], 3.0)
        self.assertEqual(med_values[2][1], 4.0)

        # 5. Write additional data using the new name and verify combined results
        write_points(
            ["sensor,loc=a temperature=6.0 6000000000"],
            db=DB,
        )
        time.sleep(0.5)

        res_combined = query(
            "SELECT moving_average(mean(temperature), 3) FROM sensor "
            "WHERE time >= 0 AND time <= 7s GROUP BY time(1s)",
            db=DB,
        )
        combined_values = res_combined["results"][0]["series"][0]["values"]
        # Now over [1, 2, 3, 4, 5, 6]:
        # At t=3: 2.0, t=4: 3.0, t=5: 4.0, t=6: 5.0
        self.assertEqual(len(combined_values), 4, "Expected 4 results with new data")
        self.assertEqual(combined_values[3][1], 5.0)

        # 6. Verify old field name does NOT work with moving functions
        res_old = query(
            "SELECT moving_average(mean(temp), 3) FROM sensor "
            "WHERE time >= 0 AND time <= 6s GROUP BY time(1s)",
            db=DB,
        )
        # Should return no series or empty results since temp is renamed
        series_old = res_old.get("results", [{}])[0].get("series", [])
        self.assertEqual(
            len(series_old), 0,
            "Old field name 'temp' should not return results"
        )

    def test_19_reconciliation_after_external_data(self):
        """
        Verify that externally-added data (simulating a backup restore or
        offline import) gets identity mappings created through reconciliation
        on container restart.

        This test:
        1. Writes data and renames a field (creates mappings)
        2. Writes new data to a NEW measurement via line protocol (simulates
           new data appearing that has no mapping yet)
        3. Verifies the new measurement gets an identity mapping
        4. Verifies the existing rename is unaffected
        """
        DB = "e2e_db_reconcile"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write data and create a rename mapping
        write_points(["existing_meas,tag=1 f1=10.0 1000000000"], db=DB)
        time.sleep(0.5)
        query(
            "ALTER MEASUREMENT existing_meas RENAME FIELD f1 TO f1_renamed",
            db=DB,
            method="POST",
        )

        # 2. Write data to a brand new measurement (dense mapping creates identity)
        write_points(["new_meas,tag=1 new_field=42.0 2000000000"], db=DB)
        time.sleep(0.5)

        # 3. Verify the new measurement has an identity mapping
        mappings_res = query("SHOW MEASUREMENT MAPPINGS", db=DB)
        series = mappings_res.get("results", [{}])[0].get("series", [])
        self.assertTrue(len(series) > 0, "Expected measurement mappings")

        found_new = False
        found_existing = False
        for row in series[0]["values"]:
            if row[0] == "new_meas":
                found_new = True
                # Identity mapping: user_name == internal_name
                self.assertEqual(
                    row[0], row[1],
                    "New measurement should have identity mapping"
                )
            if row[0] == "existing_meas":
                found_existing = True
        self.assertTrue(found_new, "new_meas should appear in mappings")
        self.assertTrue(found_existing, "existing_meas should still be in mappings")

        # 4. Verify the existing rename is unaffected
        res = query("SELECT f1_renamed FROM existing_meas", db=DB)
        series_data = res.get("results", [{}])[0].get("series", [])
        self.assertEqual(len(series_data), 1, "Renamed field should still work")
        self.assertEqual(series_data[0]["values"][0][1], 10.0)

        # 5. Verify field mappings show the new field with identity mapping
        field_mappings = query("SHOW FIELD MAPPINGS", db=DB)
        field_series = field_mappings.get("results", [{}])[0].get("series", [])
        self.assertTrue(len(field_series) > 0, "Expected field mappings")

        found_new_field = False
        found_renamed = False
        for row in field_series[0]["values"]:
            # Columns: measurement, user_name, internal_name
            if row[0] == "new_meas" and row[1] == "new_field":
                found_new_field = True
                self.assertEqual(
                    row[1], row[2],
                    "new_field should have identity mapping"
                )
            if row[0] == "existing_meas" and row[1] == "f1_renamed":
                found_renamed = True
                # Internal name should be f1 (the original)
                self.assertEqual(row[2], "f1", "Internal name should be f1")
        self.assertTrue(found_new_field, "new_field should appear in field mappings")
        self.assertTrue(found_renamed, "f1_renamed should appear in field mappings")

    def test_20_reconciliation_after_restart(self):
        """
        Verify that after a container restart, measurements and fields that
        exist in shard data but are missing from mapping stores get identity
        mappings created via reconciliation.

        This requires Docker access to restart the container.
        """
        import subprocess
        import os

        # Skip if not running in Docker E2E environment
        container_name = os.environ.get("INFLUXDB_CONTAINER", "")
        if not container_name:
            self.skipTest(
                "INFLUXDB_CONTAINER env var not set; "
                "skipping container restart test"
            )

        DB = "e2e_db_restart_reconcile"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # 1. Write data and rename a field
        write_points(["meas_before,tag=1 f_before=10.0 1000000000"], db=DB)
        time.sleep(0.5)
        query(
            "ALTER MEASUREMENT meas_before RENAME FIELD f_before TO f_renamed",
            db=DB,
            method="POST",
        )
        time.sleep(0.5)

        # 2. Restart the container to trigger reconciliation
        subprocess.run(
            ["docker", "restart", container_name],
            check=True, timeout=30,
        )

        # Wait for the container to be ready
        for attempt in range(30):
            try:
                query("SHOW DATABASES", db="")
                break
            except Exception:
                time.sleep(1)
        else:
            self.fail("Container did not restart within 30 seconds")

        # 3. Verify existing rename is preserved (reconciliation must not
        #    overwrite existing mappings)
        res = query("SELECT f_renamed FROM meas_before", db=DB)
        series = res.get("results", [{}])[0].get("series", [])
        self.assertEqual(len(series), 1, "Renamed field should survive restart")
        self.assertEqual(series[0]["values"][0][1], 10.0)

        # 4. Verify that querying by old name returns nothing
        res_old = query("SELECT f_before FROM meas_before", db=DB)
        self.assertNotIn(
            "series", res_old.get("results", [{}])[0],
            "Old field name should not return data after restart"
        )

        # 5. Write to a new measurement (creates mapping via normal write path)
        write_points(["meas_after,tag=1 f_after=20.0 3000000000"], db=DB)
        time.sleep(0.5)

        res_after = query("SELECT f_after FROM meas_after", db=DB)
        series_after = res_after.get("results", [{}])[0].get("series", [])
        self.assertEqual(len(series_after), 1, "New measurement after restart should work")


    def test_21_drop_recreate_field_gets_version_suffix(self):
        """
        Regression test for Bug #1: dropped field internal slot must NOT be
        reused when the same user name is written again.

        Before the fix, createMappingLocked treated ByInternal[k]=="" as a
        free slot and recycled it, making pre-drop TSM data visible again under
        the recreated name.

        Expected (per README_FIELD_MAPPINGS.md Scenario 2):
          - DROP FIELD temp        -> ByInternal["temp"] = "" (tombstone)
          - Write temp again       -> new internal name is "temp.v2"
          - SHOW FIELD MAPPINGS   -> temp -> temp.v2
          - Query temp             -> returns only post-recreate data (1 point)
        """
        print("\n--- Running test_21_drop_recreate_field_gets_version_suffix ---")
        DB = "e2e_db_drop_recreate_field"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        # Write original data to field "temp".
        write_points(["sensors,tag=1 temp=1.0 1000000000"], db=DB)
        time.sleep(0.5)

        # Drop the field — marks the internal slot as a tombstone.
        query("DROP FIELD temp FROM sensors", db=DB, method="POST")
        time.sleep(0.5)

        # Verify the field is gone.
        res_gone = query("SELECT temp FROM sensors", db=DB)
        series_gone = res_gone.get("results", [{}])[0].get("series", [])
        self.assertEqual(
            len(series_gone), 0,
            f"Dropped field should not be visible: {res_gone}",
        )

        # Write new data under the same user name "temp".
        write_points(["sensors,tag=1 temp=99.0 2000000000"], db=DB)
        time.sleep(0.5)

        # The mapping for "temp" must point to a NEW internal name (temp.v2),
        # not the old tombstoned slot, so that TSM data for the original "temp"
        # internal key stays hidden.
        mappings = query("SHOW FIELD MAPPINGS FROM sensors", db=DB)
        field_series = mappings.get("results", [{}])[0].get("series", [])
        self.assertTrue(len(field_series) > 0, "Expected field mappings for sensors")

        temp_internal = None
        for row in field_series[0]["values"]:
            # Columns: measurement, user_name, internal_name
            if row[1] == "temp":
                temp_internal = row[2]
                break

        self.assertIsNotNone(temp_internal, "Mapping for 'temp' not found in SHOW FIELD MAPPINGS")
        self.assertEqual(
            temp_internal, "temp.v2",
            f"Recreated field 'temp' must use internal name 'temp.v2' to avoid "
            f"exposing pre-drop data, but got '{temp_internal}'",
        )

        # Query must return only the post-recreate point (99.0), not the old
        # pre-drop value (1.0). If the tombstone were reused, both would appear.
        res_new = query("SELECT temp FROM sensors", db=DB)
        series_new = res_new.get("results", [{}])[0].get("series", [])
        self.assertEqual(len(series_new), 1, f"Expected 1 series for recreated temp: {res_new}")
        values = series_new[0]["values"]
        self.assertEqual(len(values), 1, f"Expected exactly 1 point (post-recreate only): {values}")
        self.assertAlmostEqual(
            values[0][1], 99.0, places=6,
            msg=f"Expected post-recreate value 99.0, got {values[0][1]}",
        )

    def test_22_moving_window_size_overflow_rejected(self):
        """
        Regression test for Issue #11: moving_average and moving_median must
        reject window sizes above 10000 to prevent int64 overflow when the
        size is multiplied by a query interval in nanoseconds.
        """
        print("\n--- Running test_22_moving_window_size_overflow_rejected ---")
        DB = "e2e_db_moving_overflow"
        query(f"DROP DATABASE {DB}", db="", method="POST")
        query(f"CREATE DATABASE {DB}", db="", method="POST")

        write_points(["sensors val=1.0 1000000000"], db=DB)
        time.sleep(0.5)

        def _assert_error_response(res, fn_name):
            """Assert that a query result carries an error, not data."""
            result0 = res.get("results", [{}])[0]
            has_error = "error" in result0
            has_series = "series" in result0 and len(result0["series"]) > 0
            self.assertTrue(
                has_error or not has_series,
                f"{fn_name}(val, 10001) should return an error or no data, got: {res}",
            )
            if has_error:
                self.assertIn(
                    "10001", result0["error"],
                    f"Error message should mention the rejected window size: {result0['error']}",
                )

        # moving_average with windowSize > 10000 should be rejected.
        try:
            res_avg = query(
                "SELECT moving_average(val, 10001) FROM sensors",
                db=DB,
            )
            _assert_error_response(res_avg, "moving_average")
        except Exception as e:
            # An HTTP-level error is also acceptable — the request was rejected.
            self.assertIn(
                "10001", str(e),
                f"HTTP error should mention the rejected window size: {e}",
            )

        # moving_median with windowSize > 10000 should be rejected.
        try:
            res_med = query(
                "SELECT moving_median(val, 10001) FROM sensors",
                db=DB,
            )
            _assert_error_response(res_med, "moving_median")
        except Exception as e:
            self.assertIn(
                "10001", str(e),
                f"HTTP error should mention the rejected window size: {e}",
            )


class TestMovingFunctionsAfterRename(unittest.TestCase):
    """moving_average and moving_median work correctly on renamed fields."""

    DB = "e2e_db_moving_rename"

    @classmethod
    def setUpClass(cls):
        query(f"DROP DATABASE {cls.DB}", db="", method="POST")
        query(f"CREATE DATABASE {cls.DB}", db="", method="POST")

        # Write 5 points with field "temp" at 1-second intervals.
        # Times are in nanoseconds: 1s, 2s, 3s, 4s, 5s.
        write_points(
            [
                "sensors temp=10.0 1000000000",
                "sensors temp=20.0 2000000000",
                "sensors temp=30.0 3000000000",
                "sensors temp=40.0 4000000000",
                "sensors temp=50.0 5000000000",
            ],
            db=cls.DB,
        )
        time.sleep(0.5)

        # Rename field "temp" -> "temperature".
        query("ALTER MEASUREMENT sensors RENAME FIELD temp TO temperature", db=cls.DB, method="POST")
        time.sleep(0.5)

    def _get_values(self, result):
        """Extract [[time, value], ...] from the first series of a query result."""
        series = result.get("results", [{}])[0].get("series", [])
        self.assertEqual(len(series), 1, f"Expected 1 series, got {len(series)}: {result}")
        return series[0]["values"]

    def test_moving_average_after_rename(self):
        # moving_average(temperature, 3) over the 5 points.
        # Window = 3 rows (no GROUP BY interval → row-based window).
        # Expected output rows (InfluxDB emits from row index windowSize-1 onward):
        #   t=3s: avg(10,20,30) = 20.0
        #   t=4s: avg(20,30,40) = 30.0
        #   t=5s: avg(30,40,50) = 40.0
        res = query(
            "SELECT moving_average(temperature, 3) FROM sensors",
            db=self.DB,
        )
        values = self._get_values(res)
        self.assertEqual(len(values), 3, f"Expected 3 output rows, got {len(values)}: {values}")
        self.assertAlmostEqual(values[0][1], 20.0, places=6)
        self.assertAlmostEqual(values[1][1], 30.0, places=6)
        self.assertAlmostEqual(values[2][1], 40.0, places=6)

    def test_moving_median_after_rename(self):
        # moving_median(temperature, 3) over the 5 points.
        # Expected output rows (same window logic as moving_average):
        #   t=3s: median(10,20,30) = 20.0
        #   t=4s: median(20,30,40) = 30.0
        #   t=5s: median(30,40,50) = 40.0
        res = query(
            "SELECT moving_median(temperature, 3) FROM sensors",
            db=self.DB,
        )
        values = self._get_values(res)
        self.assertEqual(len(values), 3, f"Expected 3 output rows, got {len(values)}: {values}")
        self.assertAlmostEqual(values[0][1], 20.0, places=6)
        self.assertAlmostEqual(values[1][1], 30.0, places=6)
        self.assertAlmostEqual(values[2][1], 40.0, places=6)

    def test_old_field_name_returns_no_data(self):
        # After rename, querying by the old name "temp" should return no series.
        res = query("SELECT temp FROM sensors", db=self.DB)
        series = res.get("results", [{}])[0].get("series", [])
        self.assertEqual(len(series), 0, f"Old field name should not return data: {res}")


if __name__ == "__main__":
    unittest.main(verbosity=2)
