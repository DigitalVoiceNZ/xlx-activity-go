/***
 * check - database sanity checks
 */
import { Database } from "bun:sqlite";

let onair = {}; // map of module to object with ts, id
let offair = {};
let lasttx = 0;
const GAPTIME = 6 * 60 * 60 * 1000;

const db = new Database(process.env.DBFILE);
db.exec("PRAGMA journal_mode = WAL;");
db.exec("PRAGMA busy_timeout = 5000;");
// get all the rows
const qrows = db.query("SELECT * from activity WHERE tsoff>0 ORDER BY created");
const rows = qrows.all();
console.log(`checking ${rows.length}`);

for (let row of rows) {
  let module = row.module;
  // check if this row's ts is less than tsoff
  if (row.ts > row.tsoff) {
    console.log(`${row.id} has tsoff <= ts`);
  }
  // check if this row's ts is before the last tsoff
  // for this module
  if (offair.hasOwnProperty(module) && (row.ts < offair[module].tsoff)) {
    console.log(`new tx before last rx ${offair[module].id}`);
    console.dir(row);
  }
  // check if this row's ts is the same or earlier than
  // the last
  if (onair.hasOwnProperty(module) && (row.ts <= onair[module].ts)) {
    console.log(`non monotonic ts ${onair[module].id} ${row.id}`);
  }

  // check for a long gap with no activity
  if ((lasttx !== 0) && (row.ts - lasttx > GAPTIME)) {
    console.log(`Gap of ${(row.ts-lasttx)/(60*1000)} minutes at ${row.id}`);
  }
  lasttx = row.ts;
  onair[module] = {ts: row.ts, id: row.id};
  offair[module] = {tsoff: row.tsoff, id: row.id};
}
db.close();

