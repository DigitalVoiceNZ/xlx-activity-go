import { Database } from "bun:sqlite";

let onair = {}; // map of module to last id of tx row

const db = new Database(process.env.DBFILE);
db.exec("PRAGMA journal_mode = WAL;");
db.exec("PRAGMA busy_timeout = 5000;");
const qrows = db.query("SELECT * from activity WHERE tsoff = 0 ORDER BY ts");
const rows = qrows.all();

const upd = db.query("UPDATE activity SET tsoff=?2 WHERE id=?1");

for (let row of rows) {
  if (row.tsoff !== 0) continue;
  if (row.call) {
    onair[row.module] = row.id;
  } else {
    if (row.module in onair) {
      const results = upd.all(onair[row.module], row.ts);
      delete onair[row.module];
    } else {
      console.log(`spurious rx on module ${row.module} @ ${row.ts}`)
    }
  }
}
db.close();

