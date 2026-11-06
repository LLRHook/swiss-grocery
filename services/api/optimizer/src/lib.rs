use std::collections::BTreeMap;
use std::io::Read;

pub const MAX_ROWS: usize = 100_000;
pub const MAX_INPUT: usize = 16 * 1024 * 1024;
pub const MAX_NAME: usize = 64 * 1024;
pub const MAX_OUTPUT: usize = 4 * 1024 * 1024;
const REQUEST: &[u8; 8] = b"SGOP0001";
const RESPONSE: &[u8; 8] = b"SGRS0001";

#[derive(Clone, Debug)]
pub struct Candidate {
    pub name: String,
    pub store_id: i64,
    pub price: f64,
}

#[derive(Debug, PartialEq)]
pub struct StorePlan {
    pub store_id: i64,
    pub subtotal: f64,
    pub indices: Vec<u32>,
}

#[derive(Debug, PartialEq)]
pub struct Plan {
    pub total: f64,
    pub savings: f64,
    pub stores: Vec<StorePlan>,
}

// Round half away from zero, including signed zero. Scaling and rounding are
// separate operations, as in Go's math.Round(x*100)/100 (never add 0.5 first).
fn cents(value: f64) -> Result<f64, &'static str> {
    let rounded = (value * 100.0).round() / 100.0;
    if rounded.is_finite() {
        Ok(rounded)
    } else {
        Err("non-finite total")
    }
}

pub fn optimize(rows: &[Candidate]) -> Result<Plan, &'static str> {
    if rows.len() > MAX_ROWS {
        return Err("too many rows");
    }
    // Sorted UTF-8 keys also define the addition order. Strict comparisons keep
    // the first input row on equal prices; product ID/currency are not keys.
    let mut prices: BTreeMap<&str, (usize, f64)> = BTreeMap::new();
    for (index, row) in rows.iter().enumerate() {
        if !row.price.is_finite() || row.name.len() > MAX_NAME {
            return Err("invalid candidate");
        }
        let entry = prices.entry(&row.name).or_insert((index, row.price));
        if row.price < rows[entry.0].price {
            entry.0 = index;
        }
        if row.price > entry.1 {
            entry.1 = row.price;
        }
    }

    let mut stores: BTreeMap<i64, StorePlan> = BTreeMap::new();
    let mut total = 0.0;
    let mut worst = 0.0;
    for (index, worst_price) in prices.values() {
        let row = &rows[*index];
        let store = stores.entry(row.store_id).or_insert_with(|| StorePlan {
            store_id: row.store_id,
            subtotal: 0.0,
            indices: Vec::new(),
        });
        store.indices.push(*index as u32);
        store.subtotal += row.price;
        total += row.price;
        worst += *worst_price;
    }
    let mut plans = Vec::with_capacity(stores.len());
    for mut store in stores.into_values() {
        store.subtotal = cents(store.subtotal)?;
        plans.push(store);
    }
    Ok(Plan {
        total: cents(total)?,
        savings: cents(worst - total)?,
        stores: plans,
    })
}

struct Decoder<'a>(&'a [u8]);

impl<'a> Decoder<'a> {
    fn take(&mut self, size: usize) -> Result<&'a [u8], &'static str> {
        if size > self.0.len() {
            return Err("truncated request");
        }
        let (head, tail) = self.0.split_at(size);
        self.0 = tail;
        Ok(head)
    }

    fn u32(&mut self) -> Result<u32, &'static str> {
        Ok(u32::from_le_bytes(self.take(4)?.try_into().unwrap()))
    }
}

pub fn decode(bytes: &[u8]) -> Result<Vec<Candidate>, &'static str> {
    if bytes.len() > MAX_INPUT {
        return Err("request too large");
    }
    let mut decoder = Decoder(bytes);
    if decoder.take(8)? != REQUEST {
        return Err("invalid protocol version");
    }
    let count = decoder.u32()? as usize;
    if count > MAX_ROWS || count > decoder.0.len() / 20 {
        return Err("invalid row count");
    }
    let mut rows = Vec::with_capacity(count);
    for _ in 0..count {
        let size = decoder.u32()? as usize;
        if size > MAX_NAME {
            return Err("name too large");
        }
        let name = std::str::from_utf8(decoder.take(size)?)
            .map_err(|_| "invalid UTF-8")?
            .to_owned();
        let store_id = i64::from_le_bytes(decoder.take(8)?.try_into().unwrap());
        let price = f64::from_le_bytes(decoder.take(8)?.try_into().unwrap());
        if !price.is_finite() {
            return Err("invalid price");
        }
        rows.push(Candidate {
            name,
            store_id,
            price,
        });
    }
    if !decoder.0.is_empty() {
        return Err("trailing request bytes");
    }
    Ok(rows)
}

pub fn encode(plan: &Plan) -> Result<Vec<u8>, &'static str> {
    let mut out = Vec::new();
    out.extend_from_slice(RESPONSE);
    out.extend_from_slice(&plan.total.to_le_bytes());
    out.extend_from_slice(&plan.savings.to_le_bytes());
    out.extend_from_slice(&(plan.stores.len() as u32).to_le_bytes());
    for store in &plan.stores {
        out.extend_from_slice(&store.store_id.to_le_bytes());
        out.extend_from_slice(&store.subtotal.to_le_bytes());
        out.extend_from_slice(&(store.indices.len() as u32).to_le_bytes());
        for index in &store.indices {
            out.extend_from_slice(&index.to_le_bytes());
        }
        if out.len() > MAX_OUTPUT {
            return Err("response too large");
        }
    }
    Ok(out)
}

pub fn process(reader: impl Read) -> Result<Vec<u8>, &'static str> {
    let mut bytes = Vec::new();
    reader
        .take((MAX_INPUT + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(|_| "read failed")?;
    encode(&optimize(&decode(&bytes)?)?)
}

// No unwinding crosses the process boundary. The Go host also treats aborts,
// non-zero exit, invalid output and deadline expiry as request failures.
pub fn contain(
    work: impl FnOnce() -> Result<Vec<u8>, &'static str> + std::panic::UnwindSafe,
) -> Result<Vec<u8>, &'static str> {
    std::panic::catch_unwind(work).unwrap_or(Err("optimizer panic"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn row(name: &str, store_id: i64, price: f64) -> Candidate {
        Candidate {
            name: name.into(),
            store_id,
            price,
        }
    }

    #[test]
    fn assigns_distinct_stores_and_computes_worst_savings() {
        let rows = [
            row("milk", 2, 1.2),
            row("milk", 1, 3.0),
            row("bread", 1, 2.0),
            row("bread", 2, 5.0),
        ];
        let plan = optimize(&rows).unwrap();
        assert_eq!(plan.total, 3.2);
        assert_eq!(plan.savings, 4.8);
        assert_eq!(plan.stores.len(), 2);
        assert_eq!(plan.stores[0].store_id, 1);
        assert_eq!(plan.stores[0].indices, [2]);
        assert_eq!(plan.stores[1].store_id, 2);
        assert_eq!(plan.stores[1].indices, [0]);
    }

    #[test]
    fn collapses_names_and_preserves_first_equal_offer() {
        let rows = [row("Äpfel 🛒", 9, 2.0), row("Äpfel 🛒", 3, 2.0)];
        assert_eq!(optimize(&rows).unwrap().stores[0].store_id, 9);
        let reversed: Vec<_> = rows.into_iter().rev().collect();
        assert_eq!(optimize(&reversed).unwrap().stores[0].store_id, 3);
    }

    #[test]
    fn empty_is_a_valid_plan() {
        assert_eq!(
            optimize(&[]).unwrap(),
            Plan {
                total: 0.0,
                savings: 0.0,
                stores: vec![]
            }
        );
    }

    #[test]
    fn rounds_half_away_from_zero_after_binary_scaling() {
        for (input, expected) in [(0.125, 0.13), (-0.125, -0.13), (1.005, 1.0), (2.675, 2.68)] {
            assert_eq!(cents(input).unwrap(), expected);
        }
        assert_eq!(cents(-0.004).unwrap().to_bits(), (-0.0f64).to_bits());
        assert_eq!(cents(f64::from_bits(0.125f64.to_bits() - 1)).unwrap(), 0.12);
    }

    #[test]
    fn rejects_non_finite_inputs_and_overflow() {
        for value in [f64::NAN, f64::INFINITY, f64::NEG_INFINITY, f64::MAX] {
            assert!(optimize(&[row("x", 1, value)]).is_err());
        }
    }

    #[test]
    fn rejects_invalid_frames_without_panicking() {
        for bytes in [
            vec![],
            b"SGOP9999".to_vec(),
            b"SGOP0001\x01\0\0\0".to_vec(),
            b"SGOP0001\0\0\0\0extra".to_vec(),
        ] {
            assert!(contain(|| process(bytes.as_slice())).is_err());
        }
        assert!(process(&vec![0; MAX_INPUT + 1][..]).is_err());
    }

    #[test]
    fn contains_unwinding_panics() {
        assert_eq!(
            contain(|| panic!("synthetic failure")),
            Err("optimizer panic")
        );
    }

    #[test]
    fn handles_the_row_limit_in_canonical_order() {
        let mut rows: Vec<_> = (0..MAX_ROWS)
            .map(|i| row(&format!("item-{i:06}"), (i % 11) as i64, 1.0))
            .collect();
        let plan = optimize(&rows).unwrap();
        assert_eq!(plan.total, MAX_ROWS as f64);
        assert_eq!(plan.savings, 0.0);
        assert_eq!(
            plan.stores.iter().map(|s| s.indices.len()).sum::<usize>(),
            MAX_ROWS
        );
        rows.push(row("excess", 1, 1.0));
        assert!(optimize(&rows).is_err());
    }
}
