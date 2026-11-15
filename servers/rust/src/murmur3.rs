//! MurmurHash3: x86_32 as Lucene's StringHelper and OpenSearch's
//! Murmur3HashFunction compute it (shard routing), and the first half of
//! x64_128 (cardinality hashing).

/// murmurhash3_x86_32 of data with seed, as a signed Java int.
pub fn x86_32(data: &[u8], seed: u32) -> i32 {
    const C1: u32 = 0xcc9e2d51;
    const C2: u32 = 0x1b873593;
    let mut h = seed;
    let n = data.len() & !3;
    for c in data[..n].chunks_exact(4) {
        let mut k = u32::from_le_bytes([c[0], c[1], c[2], c[3]]);
        k = k.wrapping_mul(C1).rotate_left(15).wrapping_mul(C2);
        h ^= k;
        h = h.rotate_left(13).wrapping_mul(5).wrapping_add(0xe6546b64);
    }
    let tail = &data[n..];
    if !tail.is_empty() {
        let mut k = 0u32;
        for (i, b) in tail.iter().enumerate() {
            k ^= (*b as u32) << (8 * i);
        }
        k = k.wrapping_mul(C1).rotate_left(15).wrapping_mul(C2);
        h ^= k;
    }
    h ^= data.len() as u32;
    h ^= h >> 16;
    h = h.wrapping_mul(0x85ebca6b);
    h ^= h >> 13;
    h = h.wrapping_mul(0xc2b2ae35);
    h ^= h >> 16;
    h as i32
}

/// Murmur3HashFunction.hash(String): x86_32 (seed 0) of the UTF-16 code
/// units of s, each written low byte first.
pub fn hash_routing(s: &str) -> i32 {
    let mut b = Vec::with_capacity(s.len() * 2);
    for u in s.encode_utf16() {
        b.extend_from_slice(&u.to_le_bytes());
    }
    x86_32(&b, 0)
}

/// h1 of murmurhash3_x64_128 (seed 0).
pub fn x64_128_h1(data: &[u8]) -> u64 {
    const C1: u64 = 0x87c37b91114253d5;
    const C2: u64 = 0x4cf5ad432745937f;
    let mut h1: u64 = 0;
    let mut h2: u64 = 0;
    let length = data.len();
    let nblocks = length / 16;
    for i in 0..nblocks {
        let mut k1 = u64::from_le_bytes(data[i * 16..i * 16 + 8].try_into().unwrap());
        let mut k2 = u64::from_le_bytes(data[i * 16 + 8..i * 16 + 16].try_into().unwrap());
        k1 = k1.wrapping_mul(C1);
        k1 = k1.rotate_left(31);
        k1 = k1.wrapping_mul(C2);
        h1 ^= k1;
        h1 = h1.rotate_left(27);
        h1 = h1.wrapping_add(h2);
        h1 = h1.wrapping_mul(5).wrapping_add(0x52dce729);
        k2 = k2.wrapping_mul(C2);
        k2 = k2.rotate_left(33);
        k2 = k2.wrapping_mul(C1);
        h2 ^= k2;
        h2 = h2.rotate_left(31);
        h2 = h2.wrapping_add(h1);
        h2 = h2.wrapping_mul(5).wrapping_add(0x38495ab5);
    }
    let tail = &data[nblocks * 16..];
    let mut k1: u64 = 0;
    let mut k2: u64 = 0;
    let tl = tail.len();
    for i in 0..tl.min(8) {
        k1 ^= (tail[i] as u64) << (i * 8);
    }
    for i in 8..tl {
        k2 ^= (tail[i] as u64) << ((i - 8) * 8);
    }
    if tl > 8 {
        k2 = k2.wrapping_mul(C2);
        k2 = k2.rotate_left(33);
        k2 = k2.wrapping_mul(C1);
        h2 ^= k2;
    }
    if tl > 0 {
        k1 = k1.wrapping_mul(C1);
        k1 = k1.rotate_left(31);
        k1 = k1.wrapping_mul(C2);
        h1 ^= k1;
    }
    h1 ^= length as u64;
    h2 ^= length as u64;
    h1 = h1.wrapping_add(h2);
    h2 = h2.wrapping_add(h1);
    fn fmix(mut k: u64) -> u64 {
        k ^= k >> 33;
        k = k.wrapping_mul(0xff51afd7ed558ccd);
        k ^= k >> 33;
        k = k.wrapping_mul(0xc4ceb9fe1a85ec53);
        k ^= k >> 33;
        k
    }
    h1 = fmix(h1);
    h2 = fmix(h2);
    h1 = h1.wrapping_add(h2);
    h1
}
