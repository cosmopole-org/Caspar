use blockingqueue::BlockingQueue;
use once_cell::sync::Lazy;
use rocksdb::{
    Options, ReadOptions, TransactionDB, TransactionDBOptions, TransactionOptions, WriteOptions,
};
use serde_json::{json, Value as JsonValue};
use std::collections::{BTreeMap, HashMap, VecDeque};
use std::ops::DerefMut;
use std::sync::atomic::AtomicI32;
use std::sync::atomic::{AtomicBool, AtomicI64, Ordering};
use std::sync::mpsc::{self, Receiver, Sender};
use std::sync::{Arc, Condvar, Mutex};
use std::thread;
use std::time::{Duration, Instant};
use wasmedge_sys::{
    config::Config, AsInstance, CallingFrame, Executor, Function, ImportModule, Instance, Loader,
    Statistics, Store, Validator, WasmValue,
};
use wasmedge_types::{error::CoreError, ValType};
use timedmap::TimedMap;

static RESP_MAP: Lazy<Arc<Mutex<TimedMap<i64, String>>>> =
    Lazy::new(|| Arc::new(Mutex::new(TimedMap::new())));
static TRIGGER_MAP: Lazy<Arc<Mutex<TimedMap<i64, Arc<Condvar>>>>> =
    Lazy::new(|| Arc::new(Mutex::new(TimedMap::new())));
static REQ_ID_COUNTER: Lazy<AtomicI64> = Lazy::new(|| AtomicI64::new(0));
static GLOBAL_REQ_CHAN: Lazy<BlockingQueue<String>> = Lazy::new(|| BlockingQueue::new());
static GLOBAL_TRX_STORE: Lazy<BlockingQueue<(Vec<ChainTrx>, String)>> =
    Lazy::new(|| BlockingQueue::new());
static GLOBAL_HEART_BEAT: Lazy<Arc<Condvar>> = Lazy::new(|| Arc::new(Condvar::new()));
static GLOBAL_DB: Lazy<Arc<Mutex<TransactionDB>>> = Lazy::new(|| {
    let path = "appletdb";
    let mut db_options = Options::default();
    db_options.create_if_missing(true);
    let txn_db_options = TransactionDBOptions::default();
    let db = TransactionDB::open(&db_options, &txn_db_options, path).unwrap();
    Arc::new(Mutex::new(db))
});
static mut STEP: i32 = 0;

fn main() {
    let receiver_handler = thread::spawn(|| {
        let context = zmq::Context::new();
        let responder = Arc::new(Mutex::new(context.socket(zmq::REP).unwrap()));
        {
            let res_lock = responder.lock().unwrap();
            assert!(res_lock.bind("tcp://*:5556").is_ok());
        }
        let mut msg = zmq::Message::new();
        loop {
            {
                let res_lock = responder.lock().unwrap();
                res_lock.recv(&mut msg, 0).unwrap();
            }
            let data = msg.as_str().unwrap();
            println!("recevied {data}");
            let packet: JsonValue = serde_json::from_str(data).unwrap();
            if packet["type"] == "runOffChain" {
                thread::spawn(move || {
                    let ast_path = packet["astPath"].as_str().unwrap().to_string();
                    let input = packet["input"].as_str().unwrap().to_string();
                    let machine_id = packet["machineId"].as_str().unwrap().to_string();

                    let inp1 = input.clone();
                    let input_json: JsonValue = serde_json::from_str(&inp1).unwrap();
                    let point_id = input_json["point"].as_object().unwrap()["id"]
                        .as_str()
                        .unwrap()
                        .to_string();
                    let mut rt = WasmMac::new_offchain(
                        machine_id.clone(),
                        point_id,
                        ast_path.clone(),
                        Box::new(wasm_send),
                    );
                    rt.execute_on_update(inp1);
                    rt.finalize();
                });
            } else if packet["type"] == "applyTrxEffects" {
                let j: JsonValue =
                    serde_json::from_str(packet["effects"].as_str().unwrap()).unwrap();
                let global_db = GLOBAL_DB.lock().unwrap();
                let trx = global_db.transaction();
                for op in j.as_array().unwrap().iter() {
                    if op["opType"] == "put" {
                        trx.put(op["key"].as_str().unwrap(), op["val"].as_str().unwrap())
                            .unwrap();
                    } else if op["opType"] == "del" {
                        trx.delete(op["key"].as_str().unwrap()).unwrap();
                    }
                }
                trx.commit().unwrap();
                log("applied transactions effects successfully.".to_string());
            } else if packet["type"] == "runOnChain" {
                let input: JsonValue =
                    serde_json::from_str(packet["input"].as_str().unwrap()).unwrap();
                let mut trxs: Vec<ChainTrx> = vec![];
                for item in input.as_array().unwrap().iter() {
                    trxs.push(ChainTrx::new(
                        item["machineId"].as_str().unwrap().to_string(),
                        item["key"].as_str().unwrap().to_string(),
                        item["payload"].as_str().unwrap().to_string(),
                        item["userId"].as_str().unwrap().to_string(),
                        item["callbackId"].as_str().unwrap().to_string(),
                    ));
                }
                GLOBAL_TRX_STORE.push((trxs, packet["astStorePath"].as_str().unwrap().to_string()));
            } else if packet["type"] == "apiResponse" {
                let request_id = packet["requestId"].as_i64().unwrap();
                RESP_MAP
                    .lock()
                    .unwrap()
                    .insert(request_id, packet["data"].as_str().unwrap().to_string(), Duration::from_secs(180));
                let mut tgm_lock = TRIGGER_MAP.lock().unwrap();
                let t_item = tgm_lock.get(&request_id);
                if !t_item.is_none() {
                    t_item.unwrap().notify_one();
                }
            }
            {
                let res_lock = responder.lock().unwrap();
                res_lock.send("", 0).unwrap();
            }
        }
    });
    let chan = GLOBAL_REQ_CHAN.clone();
    let sender_handler = thread::spawn(move || {
        println!("Connecting to host platform server...\n");
        let context = zmq::Context::new();
        let requester = context.socket(zmq::REQ).unwrap();
        assert!(requester.connect("tcp://localhost:5555").is_ok());
        let mut msg = zmq::Message::new();
        loop {
            let packet = chan.pop();
            requester.send(&packet, 0).unwrap();
            requester.recv(&mut msg, 0).unwrap();
        }
    });
    trx_processor();
    receiver_handler.join().unwrap();
    sender_handler.join().unwrap();
}

pub fn trx_processor() {
    let (tx, rx): (Sender<i32>, Receiver<i32>) = mpsc::channel();
    thread::spawn(move || loop {
        let block = GLOBAL_TRX_STORE.pop();
        {
            log("generating concurrent runner...".to_string());
            ConcurrentRunner::init(block.1, block.0, tx.clone());
            unsafe {
                log("running paralell transactions...".to_string());
                let mut gcr_lock = GLOBAL_CR.lock().unwrap();
                gcr_lock.run();
                drop(gcr_lock);
                log("waiting for paralell transactions to be done...".to_string());
            }
        }
        rx.recv().unwrap();
    });
}

fn wasm_send(mut data: JsonValue) -> std::string::String {
    let req_id = REQ_ID_COUNTER.fetch_add(1, Ordering::Relaxed);
    data["requestId"] = JsonValue::from(req_id);
    let cv_ = Arc::new(Condvar::new());
    {
        let cv_clone = Arc::clone(&cv_);
        let mut tgm_lock = TRIGGER_MAP.lock().unwrap();
        tgm_lock.insert(req_id, cv_clone, Duration::from_secs(180));
    }
    {
        GLOBAL_REQ_CHAN.clone().push(data.to_string());
    }
    let responses = Arc::clone(&RESP_MAP);
    let res = {
        {
            let responses_ref = responses.lock().unwrap();
            let _mg: std::sync::MutexGuard<'_, TimedMap<i64, String>> = cv_
                .wait_while(responses_ref, |res| !res.contains(&req_id))
                .unwrap();
        }
        let responses_lock: std::sync::MutexGuard<'_, TimedMap<i64, String>> =
            responses.lock().unwrap();
        let r = responses_lock.remove(&req_id);
        let r_final = if r.is_none() {
            "".to_string()
        } else {
            r.unwrap().clone()
        };
        r_final
    };
    let mut tgm_lock = TRIGGER_MAP.lock().unwrap();
    tgm_lock.remove(&req_id);
    res.to_string()
}

fn log(text: String) {
    let j = json!({
        "key": "log",
        "input": {
            "text": text
        }
    });
    wasm_send(j);
}

pub struct WasmLock {
    pub mut_: Mutex<()>,
}

impl WasmLock {
    pub fn generate() -> Self {
        WasmLock {
            mut_: Mutex::new(()),
        }
    }
}

pub struct WasmTask {
    pub id: i32,
    pub name: String,
    pub inputs: Arc<Mutex<HashMap<i32, (bool, Arc<Mutex<WasmTask>>)>>>,
    pub outputs: Arc<Mutex<HashMap<i32, Arc<Mutex<WasmTask>>>>>,
    pub vm_index: i32,
    pub started: bool,
}

impl WasmTask {
    pub fn new() -> Self {
        WasmTask {
            id: 0,
            name: String::new(),
            inputs: Arc::new(Mutex::new(HashMap::new())),
            outputs: Arc::new(Mutex::new(HashMap::new())),
            vm_index: 0,
            started: false,
        }
    }
}

pub struct ChainTrx {
    pub machine_id: String,
    pub key: String,
    pub input: String,
    pub user_id: String,
    pub callback_id: String,
}

impl ChainTrx {
    pub fn new(
        machine_id: String,
        key: String,
        input: String,
        user_id: String,
        callback_id: String,
    ) -> Self {
        ChainTrx {
            machine_id,
            key,
            input,
            user_id,
            callback_id,
        }
    }
}

#[derive(Clone)]
pub struct WasmDbOp {
    pub type_: String,
    pub key: String,
    pub val: String,
}

impl WasmDbOp {
    pub fn new() -> Self {
        WasmDbOp {
            type_: String::new(),
            key: String::new(),
            val: String::new(),
        }
    }
}

pub struct Trx {
    pub write_options: WriteOptions,
    pub read_options: ReadOptions,
    pub txn_options: TransactionOptions,
    pub store: BTreeMap<String, String>,
    pub newly_created: BTreeMap<String, bool>,
    pub newly_deleted: BTreeMap<String, bool>,
    pub ops: Vec<WasmDbOp>,
}

impl Trx {
    pub fn new() -> Self {
        Trx {
            write_options: WriteOptions::default(),
            read_options: ReadOptions::default(),
            txn_options: TransactionOptions::default(),
            store: BTreeMap::new(),
            newly_created: BTreeMap::new(),
            newly_deleted: BTreeMap::new(),
            ops: Vec::new(),
        }
    }

    pub fn get_bytes_of_str(&self, str_val: String) -> Vec<u8> {
        let mut bytes: Vec<u8> = str_val.into_bytes();
        bytes.push(0);
        bytes
    }

    pub fn put(&mut self, key: String, val: String) {
        self.ops.push(WasmDbOp {
            type_: "put".to_string(),
            key: key.clone(),
            val: val.clone(),
        });
        self.store.insert(key.clone(), val);
        self.newly_created.insert(key.clone(), true);
        self.newly_deleted.remove(&key);
    }

    pub fn get_by_prefix(&mut self, prefix: String) -> Vec<String> {
        let mut result = Vec::new();
        for (key, value) in &self.store {
            if key.starts_with(&prefix) {
                result.push(value.clone());
            }
        }
        let db = GLOBAL_DB.lock().unwrap();
        for t in db.prefix_iterator(prefix.as_bytes().to_vec().as_slice()) {
            let item = t.unwrap();
            let key = str::from_utf8(item.0.to_vec().as_slice())
                .unwrap()
                .to_string();
            let val = str::from_utf8(item.1.to_vec().as_slice())
                .unwrap()
                .to_string();
            if !self.store.contains_key(&key) && !self.store.contains_key(&key) {
                result.push(val);
            }
        }
        result
    }

    pub fn get(&mut self, key: String) -> String {
        if let Some(value) = self.store.get(&key) {
            value.clone()
        } else if self.newly_deleted.contains_key(&key) {
            return "".to_string();
        } else {
            let db = GLOBAL_DB.lock().unwrap();
            let raw_val = db.get(key.as_bytes().to_vec().as_slice());
            let value: String;
            if raw_val.is_ok() {
                value = if let Some(val) = raw_val.unwrap() {
                    str::from_utf8(val.as_slice()).unwrap().to_string()
                } else {
                    "".to_string()
                };
            } else {
                value = "".to_string();
            }
            self.store.insert(key.clone(), value.clone());
            value
        }
    }

    pub fn del(&mut self, key: String) {
        let k = String::from(key);
        self.ops.push(WasmDbOp {
            type_: "del".to_string(),
            key: k.clone(),
            val: String::new(),
        });
        self.store.remove(&k);
        self.newly_created.remove(&k);
        self.newly_deleted.insert(k, true);
    }

    pub fn commit_as_offchain(&mut self) {
        let global_db = GLOBAL_DB.lock().unwrap();
        let trx = global_db.transaction();
        for op in &self.ops {
            if op.type_ == "put" {
                trx.put(&op.key, &op.val).unwrap();
            } else if op.type_ == "del" {
                trx.delete(&op.key).unwrap();
            }
        }
        trx.commit().unwrap();
        log("committed transaction successfully.".to_string());
    }

    pub fn dummy_commit(&mut self) {}
}

pub struct WasmThreadPool {
    tasks_: Arc<Mutex<VecDeque<Box<dyn FnOnce() + Send>>>>,
    cv_: Arc<Condvar>,
    stop_: Arc<AtomicBool>,
}

impl WasmThreadPool {
    pub fn generate(num_threads: Option<usize>) -> Self {
        let num_threads = num_threads.unwrap_or_else(|| {
            thread::available_parallelism()
                .map(|n| n.get())
                .unwrap_or(1)
        });
        let vd: VecDeque<Box<dyn FnOnce() + Send>> = VecDeque::new();
        let tasks_: Arc<Mutex<VecDeque<Box<dyn FnOnce() + Send>>>> = Arc::new(Mutex::new(vd));
        let cv_ = Arc::new(Condvar::new());
        let stop_ = Arc::new(AtomicBool::new(false));

        for _i in 0..num_threads {
            let tasks_clone = Arc::clone(&tasks_);
            let cv_clone = Arc::clone(&cv_);
            let stop_clone = Arc::clone(&stop_);

            thread::spawn(move || loop {
                let task = {
                    let tasks = tasks_clone.lock().unwrap();
                    let mut _mg: std::sync::MutexGuard<'_, VecDeque<Box<dyn FnOnce() + Send>>> =
                        cv_clone
                            .wait_while(tasks, |tasks| {
                                tasks.is_empty() && !stop_clone.load(Ordering::Relaxed)
                            })
                            .unwrap();
                    if stop_clone.load(Ordering::Relaxed) && _mg.is_empty() {
                        return;
                    }

                    _mg.pop_front()
                };

                if let Some(task) = task {
                    task();
                }
            });
        }

        WasmThreadPool { tasks_, cv_, stop_ }
    }

    pub fn stop_pool(&mut self) {
        self.stop_.store(true, Ordering::Relaxed);
        self.cv_.notify_all();
    }

    pub fn enqueue<F>(&self, task: F)
    where
        F: FnOnce() + Send + 'static,
    {
        {
            let mut tasks = self.tasks_.lock().unwrap();
            tasks.push_back(Box::new(task));
            drop(tasks);
        }
        self.cv_.notify_one();
    }
}

pub struct WasmMac {
    pub onchain: bool,
    pub chain_token_id: String,
    pub token_owner_id: String,
    pub is_token_valid: bool,
    pub callback: Box<dyn (Fn(JsonValue) -> String) + Send + Sync>,
    pub machine_id: String,
    pub point_id: String,
    pub id: String,
    pub index: i32,
    pub trx: Box<Trx>,
    pub sync_tasks: Vec<SyncTask>,
    pub mod_path: String,
    pub cost: u64,

    execution_result: String,
    has_output: bool,
    tasks_: Arc<Mutex<VecDeque<(String, Sender<i32>)>>>,
    cv_: Arc<Condvar>,
    stop_: Arc<AtomicBool>,
}

pub struct HostData {
    exec: *mut Executor,
    runtime: *mut WasmMac,
}

impl WasmMac {
    pub fn new_offchain(
        machine_id: String,
        point_id: String,
        mod_path: String,
        cb: Box<dyn (Fn(JsonValue) -> String) + Send + Sync>,
    ) -> Self {
        let vd: VecDeque<(String, Sender<i32>)> = VecDeque::new();
        let tasks_: Arc<Mutex<VecDeque<(String, Sender<i32>)>>> = Arc::new(Mutex::new(vd));
        let cv_ = Arc::new(Condvar::new());
        let stop_ = Arc::new(AtomicBool::new(false));

        WasmMac {
            onchain: false,
            token_owner_id: String::new(),
            chain_token_id: String::new(),
            is_token_valid: false,
            callback: cb,
            machine_id,
            point_id,
            id: String::new(),
            index: 0,
            trx: Box::new(Trx::new()),
            sync_tasks: Vec::new(),
            mod_path,
            execution_result: "".to_string(),
            has_output: false,
            tasks_: tasks_,
            cv_: cv_,
            stop_: stop_,
            cost: 0,
        }
    }

    pub fn new_onchain(
        machine_id: String,
        vm_id: String,
        index: i32,
        mod_path: String,
        cb: Box<dyn (Fn(JsonValue) -> String) + Send + Sync>,
    ) -> Self {
        let vd: VecDeque<(String, Sender<i32>)> = VecDeque::new();
        let tasks_: Arc<Mutex<VecDeque<(String, Sender<i32>)>>> = Arc::new(Mutex::new(vd));
        let cv_ = Arc::new(Condvar::new());
        let stop_ = Arc::new(AtomicBool::new(false));

        WasmMac {
            onchain: true,
            token_owner_id: String::new(),
            chain_token_id: String::new(),
            is_token_valid: false,
            callback: cb,
            machine_id,
            point_id: String::new(),
            id: vm_id,
            index,
            trx: Box::new(Trx::new()),
            sync_tasks: Vec::new(),
            mod_path,
            execution_result: "".to_string(),
            has_output: false,
            tasks_: tasks_,
            cv_: cv_,
            stop_: stop_,
            cost: 0,
        }
    }

    pub fn finalize(&mut self) -> Vec<WasmDbOp> {
        if self.onchain {
            self.trx.dummy_commit();
        } else {
            self.trx.commit_as_offchain();
        }

        self.trx.ops.clone()
    }

    pub fn execute_on_update(&mut self, input: String) {
        let mut config = Config::create().unwrap();
        config.measure_cost(true);
        let stats = Statistics::create().unwrap();
        let mut store = Store::create().unwrap();

        let wasi_mod = wasmedge_sys::WasiModule::create(None, None, None).unwrap();

        let mut dummy: i32 = 1;
        let extern_mod = &mut ImportModule::create("env", Box::new(&mut dummy)).unwrap();

        let mut exec = Executor::create(Some(&config), Some(&stats)).unwrap();

        extern_mod.add_func("newSyncTask", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                new_sync_task,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("runDocker", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                run_docker,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("execDocker", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                exec_docker,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("copyToDocker", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                copy_to_docker,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("consoleLog", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                console_log,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("output", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                output,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("httpPost", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                http_post,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("plantTrigger", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                plant_trigger,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("signalPoint", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                signal_point,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("submitOnchainTrx", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                submit_onchain_trx,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("put", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![ValType::I32, ValType::I32, ValType::I32, ValType::I32],
                    vec![ValType::I64],
                ),
                trx_put,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("del", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                trx_del,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("get", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                trx_get,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("getByPrefix", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                trx_get_by_prefix,
                &mut (HostData {
                    exec: &mut exec,
                    runtime: self,
                }),
                1,
            )
            .unwrap()
        });

        exec.register_import_module(&mut store, &wasi_mod).unwrap();
        exec.register_import_module(&mut store, extern_mod).unwrap();

        let conf = Config::create().unwrap();
        let loader = Loader::create(Some(&conf)).unwrap();
        let main_mod_raw = loader.from_file(self.mod_path.clone()).unwrap();
        let conf2 = Config::create().unwrap();
        let v = Validator::create(Some(&conf2)).unwrap();
        v.validate(&main_mod_raw).unwrap();

        let vm_instance_res = exec.register_active_module(&mut store, &main_mod_raw);
        if vm_instance_res.is_ok() {
            let mut vm_instance = vm_instance_res.unwrap();

            let mut binding = vm_instance.get_func_mut("_start").unwrap();

            exec.call_func(&mut binding, []).unwrap();

            let val_l = input.len() as i32;
            let mut malloc_fn = vm_instance.get_func_mut("malloc").unwrap();
            let res2 = exec
                .call_func(&mut malloc_fn, [WasmValue::from_i32(val_l)])
                .unwrap();

            let val_offset = res2[0].to_i32();
            let raw_arr = input.as_bytes();
            let arr: Vec<u8> = raw_arr.to_vec();
            let mem = vm_instance.get_memory_mut("memory");
            mem.unwrap()
                .set_data(arr, val_offset.cast_unsigned())
                .unwrap();
            let c = ((val_offset as i64) << 32) | (val_l as i64);

            let mut run_fn = vm_instance.get_func_mut("run").unwrap();
            let res = exec.call_func(&mut run_fn, [WasmValue::from_i64(c)]);
            if res.is_ok() {
                res.unwrap();
            }
        }
    }

    pub fn stop(&mut self) {
        self.stop_.store(true, Ordering::Relaxed);
        self.cv_.notify_all();
    }

    pub fn enqueue_task(&self, task_id: String, cb_chan: Sender<i32>) {
        {
            let mut tasks = self.tasks_.lock().unwrap();
            tasks.push_back((task_id, cb_chan));
            drop(tasks);
        }
        self.cv_.notify_one();
    }
}

fn execute_on_chain(mac_item: Arc<Mutex<WasmMac>>, input: String, user_id: String) {
    let tasks_clone: Arc<Mutex<VecDeque<(String, Sender<i32>)>>>;
    let cv_clone: Arc<Condvar>;
    let stop_clone: Arc<AtomicBool>;
    {
        let mut mac_lock = mac_item.lock().unwrap();
        let mac = mac_lock.deref_mut();

        tasks_clone = Arc::clone(&mac.tasks_);
        cv_clone = Arc::clone(&mac.cv_);
        stop_clone = Arc::clone(&mac.stop_);

        drop(mac_lock);
    }
    thread::spawn(move || {
        let mut stats = Statistics::create().unwrap();
        let mut exec: Executor;
        let mut vm_instance: Instance;

        let gas_limit;
        let mut mac_lock = mac_item.lock().unwrap();
        let mac = mac_lock.deref_mut();

        let input_json: JsonValue = serde_json::from_str(&input).unwrap();
        let token_id = input_json["tokenId"].as_str().unwrap().to_string();
        let params = input_json["params"].as_str().unwrap().to_string();

        mac.token_owner_id = user_id.clone();
        mac.chain_token_id = token_id.clone();

        let j = json!({
            "key": "checkTokenValidity",
            "input": {
                "tokenOwnerId": user_id,
                "tokenId": token_id
            }
        });

        let val = (mac.callback)(j);

        let jsn_result = serde_json::from_str(&val);
        if jsn_result.is_ok() {
            let jsn: JsonValue = jsn_result.unwrap();
            gas_limit = jsn["gasLimit"].as_u64().unwrap_or(0);
        } else {
            gas_limit = 0;
        }

        if gas_limit == 0 {
            mac.has_output = false;
            mac.is_token_valid = false;
            drop(mac_lock);
            thread::spawn(|| {
                let cr_cloned = Arc::clone(unsafe { &GLOBAL_CR });
                let mut cr = cr_cloned.lock().unwrap();
                cr.wasm_done_tasks += 1;
                if cr.wasm_done_tasks == cr.wasm_count {
                    log("all transactions completed !".to_string());
                    unsafe {
                        if STEP == 0 {
                            cr.wasm_done_tasks = 0;
                            STEP += 1;
                            log("ready to execute critical scope...".to_string());
                            cr.wasm_do_critical();
                        }
                    }
                }
            });
            return;
        }

        mac.is_token_valid = true;

        let mut store = Store::create().unwrap();

        let wasi_mod = wasmedge_sys::WasiModule::create(None, None, None).unwrap();

        let mut dummy: i32 = 1;
        let extern_mod = &mut ImportModule::create("env", Box::new(&mut dummy)).unwrap();

        let mut config = Config::create().unwrap();
        config.measure_cost(true);
        stats.set_cost_limit(gas_limit);
        exec = Executor::create(Some(&config), Some(&stats)).unwrap();

        extern_mod.add_func("newSyncTask", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                new_sync_task,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("runDocker", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                run_docker,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("execDocker", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                exec_docker,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("copyToDocker", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                copy_to_docker,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("consoleLog", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                console_log,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("output", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                output,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("httpPost", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                http_post,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("plantTrigger", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                plant_trigger,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("signalPoint", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                signal_point,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("submitOnchainTrx", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                        ValType::I32,
                    ],
                    vec![ValType::I64],
                ),
                submit_onchain_trx,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("put", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(
                    vec![ValType::I32, ValType::I32, ValType::I32, ValType::I32],
                    vec![ValType::I64],
                ),
                trx_put,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("del", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                trx_del,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("get", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                trx_get,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });
        extern_mod.add_func("getByPrefix", unsafe {
            Function::create_sync_func(
                &wasmedge_sys::FuncType::new(vec![ValType::I32, ValType::I32], vec![ValType::I64]),
                trx_get_by_prefix,
                &mut (HostData {
                    runtime: mac,
                    exec: &mut exec,
                }),
                1,
            )
            .unwrap()
        });

        exec.register_import_module(&mut store, &wasi_mod).unwrap();
        exec.register_import_module(&mut store, extern_mod).unwrap();

        let loader = Loader::create(Some(&config)).unwrap();
        let main_mod_raw = loader.from_file(mac.mod_path.clone()).unwrap();
        let config2 = Config::create().unwrap();
        let v = Validator::create(Some(&config2)).unwrap();
        v.validate(&main_mod_raw).unwrap();

        vm_instance = exec
            .register_active_module(&mut store, &main_mod_raw)
            .unwrap();

        let mut binding = vm_instance.get_func_mut("_start").unwrap();

        exec.call_func(&mut binding, []).unwrap();

        let val_l = input.len() as i32;
        let mut malloc_fn = vm_instance.get_func_mut("malloc").unwrap();
        let res2 = exec
            .call_func(&mut malloc_fn, [WasmValue::from_i32(val_l)])
            .unwrap();

        let val_offset = res2[0].to_i32();
        let raw_arr = params.as_bytes();
        let arr: Vec<u8> = raw_arr.to_vec();
        let mem = vm_instance.get_memory_mut("memory");
        mem.unwrap()
            .set_data(arr, val_offset.cast_unsigned())
            .unwrap();
        let c = ((val_offset as i64) << 32) | (val_l as i64);

        let mut run_fn = vm_instance.get_func_mut("run").unwrap();
        let res = exec.call_func(&mut run_fn, [WasmValue::from_i64(c)]);
        if res.is_ok() {
            res.unwrap();
        }

        mac_lock.cost = stats.cost_in_total();

        log("finished a trx !".to_string());

        drop(mac_lock);

        thread::spawn(|| {
            let cr_cloned = Arc::clone(unsafe { &GLOBAL_CR });
            let mut cr = cr_cloned.lock().unwrap();
            cr.wasm_done_tasks += 1;
            if cr.wasm_done_tasks == cr.wasm_count {
                log("all transactions completed !".to_string());
                unsafe {
                    if STEP == 0 {
                        cr.wasm_done_tasks = 0;
                        STEP += 1;
                        log("ready to execute critical scope...".to_string());
                        cr.wasm_do_critical();
                    }
                }
            }
        });

        loop {
            let task = {
                let tasks = tasks_clone.lock().unwrap();
                let mut _mg: std::sync::MutexGuard<'_, VecDeque<(String, Sender<i32>)>> = cv_clone
                    .wait_while(tasks, |tasks| {
                        tasks.is_empty() && !stop_clone.load(Ordering::Relaxed)
                    })
                    .unwrap();
                if stop_clone.load(Ordering::Relaxed) && _mg.is_empty() {
                    log(
                        "------------------------------ end of vm lifecycle ---------------------------------------".to_string()
                    );
                    let mut ml = mac_item.lock().unwrap();
                    ml.cost = stats.cost_in_total();
                    return;
                }
                _mg.pop_front()
            };

            if let Some(task_id) = task {
                let val_l = task_id.0.len() as i32;

                let mut malloc_fn = vm_instance.get_func_mut("malloc").unwrap();
                let res2 = exec
                    .call_func(&mut malloc_fn, [WasmValue::from_i32(val_l)])
                    .unwrap();

                let val_offset = res2[0].to_i32();
                let raw_arr = task_id.0.as_bytes();
                let arr: Vec<u8> = raw_arr.to_vec();
                let mem = vm_instance.get_memory_mut("memory");
                mem.unwrap()
                    .set_data(arr, val_offset.cast_unsigned())
                    .unwrap();
                let c = ((val_offset as i64) << 32) | (val_l as i64);

                let mut run_fn = vm_instance.get_func_mut("runTask").unwrap();
                let res3 = exec.call_func(&mut run_fn, [WasmValue::from_i64(c)]);
                if res3.is_ok() {
                    res3.unwrap();
                }
                task_id.1.send(1).unwrap();
            }
        }
    });
}

// Sync task structure
#[derive(Clone)]
pub struct SyncTask {
    pub deps: Vec<String>,
    pub name: String,
}

pub fn new_sync_task(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_ref(0).unwrap();

    let key_offset = _input[0].to_i32();
    let key_l = _input[1].to_i32();
    let text_bytes_vec = mem
        .get_data(key_offset.cast_unsigned(), key_l.cast_unsigned())
        .unwrap();
    let text_bytes = text_bytes_vec.as_slice();
    let text = str::from_utf8(&text_bytes).unwrap();

    let j: JsonValue = serde_json::from_str(&text).unwrap_or_default();

    let name = j["name"].as_str().unwrap_or("").to_string();

    let mut deps = Vec::<String>::new();
    if let Some(deps_array) = j["deps"].as_array() {
        for item in deps_array {
            if let Some(dep_str) = item.as_str() {
                deps.push(dep_str.to_string());
            }
        }
    }

    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    rt.sync_tasks.push(SyncTask { deps, name });

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn output(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_ref(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let key_offset = _input[0].to_i32();
    let key_l = _input[1].to_i32();
    let text_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let text_bytes_next = text_bytes.unwrap();
    let text = str::from_utf8(&text_bytes_next).unwrap();

    rt.execution_result = text.to_string();
    rt.has_output = true;

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn console_log(
    _: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_ref(0).unwrap();

    let key_offset = _input[0].to_i32();
    let key_l = _input[1].to_i32();
    let text_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let text_bytes_next = text_bytes.unwrap();
    let text = str::from_utf8(&text_bytes_next).unwrap();

    log(text.to_string());

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn submit_onchain_trx(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_ref(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let tm_offset = _input[0].to_i32();
    let tm_l = _input[1].to_i32();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let key = str::from_utf8(&key_bytes_next).unwrap();

    let input_offset = _input[4].to_i32();
    let input_l = _input[5].to_i32();
    let input_bytes = mem.get_data(input_offset.cast_unsigned(), input_l.cast_unsigned());
    let input_bytes_next = input_bytes.unwrap();
    let input = str::from_utf8(&input_bytes_next).unwrap();

    let meta_offset = _input[6].to_i32();
    let meta_l = _input[7].to_i32();
    let meta_bytes = mem.get_data(meta_offset.cast_unsigned(), meta_l.cast_unsigned());
    let meta_bytes_next = meta_bytes.unwrap();
    let meta = str::from_utf8(&meta_bytes_next).unwrap();

    let is_base = meta.chars().nth(0).unwrap_or('0') == '1';
    let is_file = meta.chars().nth(1).unwrap_or('0') == '1';

    let tag = meta.chars().skip(2).collect::<String>();

    let target_machine_id = if !is_base {
        str::from_utf8(
            &mem.get_data(tm_offset.cast_unsigned(), tm_l.cast_unsigned())
                .unwrap(),
        )
        .unwrap()
        .to_string()
    } else {
        "".to_string()
    };

    let j = json!({
        "key": "submitOnchainTrx",
        "input": {
            "pointId": rt.point_id,
            "machineId": rt.machine_id,
            "targetMachineId": target_machine_id,
            "key": key,
            "tag": tag,
            "packet": input,
            "isFile": is_file,
            "isBase": is_base,
            "isRequesterOnchain": rt.onchain
        }
    });

    let val = wasm_send(j);
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn plant_trigger(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let tag = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let text = str::from_utf8(&key_bytes_next).unwrap();

    let pi_offset = _input[4].to_i32();
    let pi_l = _input[5].to_i32();
    let pi_bytes = mem.get_data(pi_offset.cast_unsigned(), pi_l.cast_unsigned());
    let pi_bytes_next = pi_bytes.unwrap();
    let point_id = str::from_utf8(&pi_bytes_next).unwrap();

    let count = _input[6].to_i32();

    let j = json!({
        "key": "plantTrigger",
        "input": {
            "machineId": rt.machine_id,
            "pointId": point_id,
            "input": text,
            "tag": tag,
            "count": count
        }
    });

    (rt.callback)(j);

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn http_post(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let url_offset = _input[0].to_i32();
    let url_l = _input[1].to_i32();
    let url_bytes = mem.get_data(url_offset.cast_unsigned(), url_l.cast_unsigned());
    let url_bytes_next = url_bytes.unwrap();
    let url = str::from_utf8(&url_bytes_next).unwrap();

    let heads_offset = _input[2].to_i32();
    let heads_l = _input[3].to_i32();
    let heads_bytes = mem.get_data(heads_offset.cast_unsigned(), heads_l.cast_unsigned());
    let heads_bytes_next = heads_bytes.unwrap();
    let headers = str::from_utf8(&heads_bytes_next).unwrap();

    let body_offset = _input[4].to_i32();
    let body_l = _input[5].to_i32();
    let body_bytes = mem.get_data(body_offset.cast_unsigned(), body_l.cast_unsigned());
    let body_bytes_next = body_bytes.unwrap();
    let body = str::from_utf8(&body_bytes_next).unwrap();

    let j = json!({
        "key": "httpPost",
        "input": {
            "machineId": rt.machine_id,
            "url": url,
            "headers": headers,
            "body": body
        }
    });

    let val = (rt.callback)(j);
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn run_docker(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let mem = _inst.get_memory_mut("memory").unwrap();

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let image_name = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let text = str::from_utf8(&key_bytes_next).unwrap();

    let cn_offset = _input[4].to_i32();
    let cn_l = _input[5].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let container_name = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "runDocker",
        "input": {
            "machineId": rt.machine_id,
            "pointId": rt.point_id,
            "inputFiles": text,
            "imageName": image_name,
            "containerName": container_name
        }
    });

    let val = (rt.callback)(j);

    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn exec_docker(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let image_name = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let container_name = str::from_utf8(&key_bytes_next).unwrap();

    let cn_offset = _input[4].to_i32();
    let cn_l = _input[5].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let command = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "execDocker",
        "input": {
            "machineId": rt.machine_id,
            "imageName": image_name,
            "containerName": container_name,
            "command": command
        }
    });

    let res = (rt.callback)(j);

    let jres = json!({
        "data": res
    });

    let val = jres.to_string();
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn copy_to_docker(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let image_name = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let container_name = str::from_utf8(&key_bytes_next).unwrap();

    let co_offset = _input[4].to_i32();
    let co_l = _input[5].to_i32();
    let co_bytes = mem.get_data(co_offset.cast_unsigned(), co_l.cast_unsigned());
    let co_bytes_next = co_bytes.unwrap();
    let file_name = str::from_utf8(&co_bytes_next).unwrap();

    let cn_offset = _input[6].to_i32();
    let cn_l = _input[7].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let content = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "copyToDocker",
        "input": {
            "machineId": rt.machine_id,
            "imageName": image_name,
            "containerName": container_name,
            "fileName": file_name,
            "content": content
        }
    });

    let res = (rt.callback)(j);

    let jres = json!({
        "data": res
    });

    let val = jres.to_string();
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn signal_point(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let typ = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let point_id = str::from_utf8(&key_bytes_next).unwrap();

    let co_offset = _input[4].to_i32();
    let co_l = _input[5].to_i32();
    let co_bytes = mem.get_data(co_offset.cast_unsigned(), co_l.cast_unsigned());
    let co_bytes_next = co_bytes.unwrap();
    let user_id = str::from_utf8(&co_bytes_next).unwrap();

    let cn_offset = _input[6].to_i32();
    let cn_l = _input[7].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let payload = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "signalPoint",
        "input": {
            "machineId": rt.machine_id,
            "type": typ,
            "pointId": point_id,
            "userId": user_id,
            "data": payload
        }
    });

    let val = (rt.callback)(j);
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn trx_put(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let key = str::from_utf8(&in_bytes_next).unwrap();

    let val_offset = _input[2].to_i32();
    let val_l = _input[3].to_i32();
    let val_bytes = mem.get_data(val_offset.cast_unsigned(), val_l.cast_unsigned());
    let val_bytes_next = val_bytes.unwrap();
    let val = str::from_utf8(&val_bytes_next).unwrap();

    rt.trx
        .put(format!("{}::{}", rt.machine_id, key), val.to_string());

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn trx_del(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let key = str::from_utf8(&in_bytes_next).unwrap();

    rt.trx.del(format!("{}::{}", rt.machine_id, key));

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn trx_get(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let key = str::from_utf8(&in_bytes_next).unwrap();

    let val = rt.trx.get(format!("{}::{}", rt.machine_id, key));
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn trx_get_by_prefix(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let prefix = str::from_utf8(&in_bytes_next).unwrap();

    let vals = rt
        .trx
        .get_by_prefix(format!("{}::{}", rt.machine_id, prefix));
    let j = json!({
        "data": vals
    });

    let val = j.to_string();
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub struct ConcurrentRunner {
    pub end_cb: Sender<i32>,
    pub wasm_done_tasks: i32,
    pub wasm_count: i32,
    pub trxs: Vec<ChainTrx>,
    pub ast_store_path: String,
    pub wasm_vms: Vec<Option<Arc<Mutex<WasmMac>>>>,
    pub wasm_vm_map: HashMap<String, (usize, Arc<Mutex<WasmMac>>)>,
    pub thread_pool: Arc<Mutex<WasmThreadPool>>,
    pub saved_key_counter: i32,
}

static mut GLOBAL_CR: Lazy<Arc<Mutex<ConcurrentRunner>>> = Lazy::new(|| {
    let (tx, _): (Sender<i32>, Receiver<i32>) = mpsc::channel();
    Arc::new(Mutex::new(ConcurrentRunner {
        end_cb: tx.clone(),
        wasm_done_tasks: 0,
        wasm_count: 0,
        trxs: vec![],
        ast_store_path: "".to_string(),
        wasm_vms: vec![],
        wasm_vm_map: HashMap::new(),
        thread_pool: Arc::new(Mutex::new(WasmThreadPool::generate(Some(0)))),
        saved_key_counter: 0,
    }))
});

impl ConcurrentRunner {
    pub fn init(ast_store_path: String, trxs: Vec<ChainTrx>, end_cb: Sender<i32>) {
        unsafe {
            let gcr_raw = Arc::clone(&GLOBAL_CR);
            let mut gcr = gcr_raw.lock().unwrap();
            gcr.trxs = trxs;
            gcr.ast_store_path = ast_store_path;
            gcr.wasm_done_tasks = 0;
            gcr.wasm_count = 0;
            gcr.wasm_vms = vec![];
            gcr.wasm_vm_map = HashMap::new();
            gcr.saved_key_counter = 0;
            gcr.end_cb = end_cb;
        }
    }

    pub fn run(&mut self) {
        self.prepare_context(self.trxs.len());
        for i in 0..self.trxs.len() {
            let trx = self.trxs[i].clone();
            let ast_store_path = self.ast_store_path.clone();
            thread::spawn(move || {
                let rt = WasmMac::new_onchain(
                    trx.machine_id.clone(),
                    i.to_string(),
                    i as i32,
                    format!("{}/{}/module", ast_store_path, trx.machine_id),
                    Box::new(wasm_send),
                );
                let rt_arc = Arc::new(Mutex::new(rt));
                {
                    unsafe {
                        let mut gcr = GLOBAL_CR.lock().unwrap();
                        let rt_cloned = Arc::clone(&rt_arc);
                        gcr.register_wasm_mac(rt_cloned);
                        drop(gcr);
                    }
                }
                execute_on_chain(rt_arc, trx.input, trx.user_id);
            });
        }
    }

    pub fn collect_results(&mut self) {
        for rt_opt in &self.wasm_vms {
            if let Some(rt_arc) = rt_opt {
                let mut rt = rt_arc.lock().unwrap();
                rt.stop();

                let cost = rt.cost;

                let output = rt.execution_result.clone();
                let changes = rt.finalize();

                let mut arr = Vec::new();
                for op in changes {
                    let item = json!({
                        "opType": op.type_,
                        "key": op.key,
                        "val": op.val
                    });
                    arr.push(item);
                }
                let ops_str = json!(arr).to_string();

                let j = json!({
                    "key": "submitOnchainResponse",
                    "input": {
                        "callbackId": self.trxs.get(rt.index as usize).unwrap().callback_id,
                        "packet": output,
                        "resCode": 1,
                        "error": "",
                        "changes": ops_str,
                        "cost": cost,
                        "tokenId": rt.chain_token_id,
                        "tokenOwnerId": rt.token_owner_id,
                    }
                });

                wasm_send(j);
            }
        }
        self.cleanup();
    }

    pub fn cleanup(&mut self) {
        self.wasm_done_tasks = 0;
        self.wasm_count = 0;
        self.trxs.clear();
        self.ast_store_path = "".to_string();
        self.wasm_vms.clear();
        self.wasm_vm_map.clear();
        self.thread_pool = Arc::new(Mutex::new(WasmThreadPool::generate(Some(0))));
        self.saved_key_counter = 0;
        self.end_cb.send(1).unwrap();
    }

    pub fn prepare_context(&mut self, vm_count: usize) {
        unsafe {
            STEP = 0;
        }
        self.wasm_count = vm_count as i32;
        self.wasm_vms = vec![None; vm_count];
    }

    pub fn register_wasm_mac(&mut self, rt: Arc<Mutex<WasmMac>>) {
        let rt_guard_lock = rt.lock().unwrap();
        let id = rt_guard_lock.id.clone();
        let index = rt_guard_lock.index;
        drop(rt_guard_lock);

        let cloned_rt1 = Arc::clone(&rt);
        let cloned_rt2 = Arc::clone(&rt);

        self.wasm_vm_map.insert(id, (index as usize, cloned_rt1));
        self.wasm_vms[index as usize] = Some(cloned_rt2);
    }

    pub fn wasm_do_critical(&mut self) {
        let mut key_counter = 1;
        let mut all_wasm_tasks: HashMap<i32, (Vec<String>, usize, String)> = HashMap::new();
        let mut task_refs: HashMap<String, Arc<Mutex<WasmTask>>> = HashMap::new();
        let mut res_wasm_locks: HashMap<String, (Option<Arc<Mutex<WasmTask>>>, Arc<WasmLock>)> =
            HashMap::new();
        let mut start_points: HashMap<String, Arc<Mutex<WasmTask>>> = HashMap::new();
        let mut res_counter = 0;

        log("iterating over vms...".to_string());

        for index in 0..self.wasm_count {
            if let Some(vm_arc) = &self.wasm_vms[index as usize] {
                log(format!("checking vm {vm_arc_id}...", vm_arc_id = index));
                let vm = vm_arc.lock().unwrap();
                for t in &vm.sync_tasks {
                    log(format!(
                        "checking sync task {task_name}...",
                        task_name = t.name
                    ));
                    let res_nums = t.deps.clone();
                    let name = t.name.clone();

                    for r in &res_nums {
                        log(format!("checking resource {r}..."));
                        if !res_wasm_locks.contains_key(r) {
                            res_wasm_locks
                                .insert(r.clone(), (None, Arc::new(WasmLock::generate())));
                            res_counter += 1;
                        }
                    }
                    all_wasm_tasks.insert(key_counter, (res_nums, index as usize, name));
                    key_counter += 1;
                }
            }
        }

        log("iterating upto key counter".to_string());

        for i in 1..=key_counter {
            if let Some((res_nums, vm_index, name)) = all_wasm_tasks.get(&i) {
                log(format!("generating wasm_task {i}..."));

                let inputs = Arc::new(Mutex::new(HashMap::new()));
                let outputs = Arc::new(Mutex::new(HashMap::new()));
                let task = Arc::new(Mutex::new(WasmTask {
                    id: i,
                    name: name.clone(),
                    inputs: inputs,
                    outputs: outputs,
                    vm_index: *vm_index as i32,
                    started: false,
                }));

                let vm_index_str = vm_index.to_string();
                let cloned_task = Arc::clone(&task);
                task_refs.insert(format!("{}:{}", vm_index_str, name), cloned_task);

                for r in res_nums {
                    log(format!("check resource {r} details..."));

                    if r.starts_with("lock_") {
                        log("resource is a lock".to_string());
                        if let Some(t) = task_refs.get(&format!("{}:{}", vm_index_str, r)) {
                            let t_cloned = Arc::clone(&t);
                            let t_cloned_ref = t_cloned.lock().unwrap();
                            let inputs_cloned = Arc::clone(&t_cloned_ref.inputs);
                            let mut inputs_cloned_ref = inputs_cloned.lock().unwrap();
                            let t_cloned2 = Arc::clone(&t);
                            inputs_cloned_ref.insert(t_cloned_ref.id, (false, t_cloned2));
                            let outputs_cloned = Arc::clone(&t_cloned_ref.outputs);
                            let mut outputs_cloned_ref = outputs_cloned.lock().unwrap();
                            let t_cloned3 = Arc::clone(&t);
                            outputs_cloned_ref.insert(t_cloned_ref.id.clone(), t_cloned3);
                        }
                    } else {
                        log("resource is a data".to_string());
                        if let Some((first_task, _)) = res_wasm_locks.get_mut(r) {
                            if first_task.is_none() {
                                log("first task is empty".to_string());
                                let cloned_task = Arc::clone(&task);
                                *first_task = Some(cloned_task);
                                let cloned_task2 = Arc::clone(&task);
                                start_points.insert(r.clone(), cloned_task2);
                                log("done 1".to_string());
                            } else {
                                log("first task is not empty".to_string());
                                let cloned_task2 = Arc::clone(&task);
                                let cloned_task_ref2 = cloned_task2.lock().unwrap();
                                let inputs_cloned = Arc::clone(&cloned_task_ref2.inputs);
                                let mut inputs_cloned_ref = inputs_cloned.lock().unwrap();
                                let item = first_task.as_ref().unwrap();
                                let t_cloned = Arc::clone(&item);
                                let t_cloned_ref = t_cloned.lock().unwrap();
                                let t_cloned_t = Arc::clone(&item);
                                inputs_cloned_ref.insert(t_cloned_ref.id, (false, t_cloned_t));
                                let outputs_cloned = Arc::clone(&t_cloned_ref.outputs);
                                let mut outputs_cloned_ref = outputs_cloned.lock().unwrap();
                                let cloned_task4 = Arc::clone(&task);
                                outputs_cloned_ref
                                    .insert(cloned_task_ref2.id.clone(), cloned_task4);
                                let cloned_task5 = Arc::clone(&task);
                                let l = &res_wasm_locks.get(r).unwrap().1;
                                let l_cloned = Arc::clone(l);
                                res_wasm_locks
                                    .insert(r.clone().to_string(), (Some(cloned_task5), l_cloned));
                                log("done 2".to_string());
                            }
                        }
                    }
                }
            }
        }

        log("generating thread pool...".to_string());

        if start_points.is_empty() {
            log("finishing and stopping parallel transactions thread...".to_string());
            self.collect_results();
        } else {
            self.thread_pool = Arc::new(Mutex::new(WasmThreadPool::generate(Some(res_counter))));
            self.saved_key_counter = key_counter;

            for (_r, task) in start_points {
                self.exec_wasm_task(task);
            }

            log("checking...".to_string());
        }
    }

    pub fn exec_wasm_task(&mut self, task: Arc<Mutex<WasmTask>>) {
        log("ok 1".to_string());
        unsafe {
            let wasm_done_wasm_tasks_count = Arc::new(AtomicI32::new(1));
            let mut ready_to_exec = false;
            {
                log("ok 2".to_string());
                let cloned_task_t = Arc::clone(&task);
                let mut cloned_task_ref_t = cloned_task_t.lock().unwrap();
                log("ok 5".to_string());
                if cloned_task_ref_t.started == false {
                    log("ok 6".to_string());
                    let mut passed = true;
                    {
                        let tc = cloned_task_ref_t.inputs.lock().unwrap();
                        for (_, val) in tc.iter() {
                            log(format!("ok 7 ... {val_bool}", val_bool = val.0));
                            if !val.0 {
                                passed = false;
                                break;
                            }
                        }
                    }
                    if passed {
                        log(format!("ok 8 ..."));
                        cloned_task_ref_t.started = true;
                        ready_to_exec = true;
                    }
                }
            }

            log(format!("ok 9 ..."));

            if ready_to_exec {
                log(format!("ok 10 ..."));

                self.thread_pool.lock().unwrap().enqueue(move || {
                    {
                        log(format!("ok 12 ..."));

                        let cloned_cr1 = Arc::clone(&GLOBAL_CR);
                        let cloned_cr_ref1 = cloned_cr1.lock().unwrap();

                        log(format!("ok 13 ..."));

                        let cloned_task_t = Arc::clone(&task);
                        let cloned_task_ref_t = cloned_task_t.lock().unwrap();

                        log(format!("ok 14 ..."));

                        let vmi = cloned_task_ref_t.vm_index.clone() as usize;

                        if let Some(rt_arc) = &cloned_cr_ref1.wasm_vms[vmi] {
                            let (tx, rx): (Sender<i32>, Receiver<i32>) = mpsc::channel();
                            let thread_tx: Sender<i32> = tx.clone();
                            {
                                let rt = rt_arc.lock().unwrap();
                                rt.enqueue_task(cloned_task_ref_t.name.clone(), thread_tx);
                                drop(rt);
                            }
                            rx.recv().unwrap();
                        }
                    }
                    let mut next_wasm_tasks: Vec<Arc<Mutex<WasmTask>>> = Vec::new();
                    {
                        log(format!("ok 18 ..."));

                        let cloned_cr3 = Arc::clone(&GLOBAL_CR);
                        let mut cloned_cr_ref3 = cloned_cr3.lock().unwrap();

                        log(format!("ok 19 ..."));

                        wasm_done_wasm_tasks_count.fetch_add(1, Ordering::Relaxed);
                        if wasm_done_wasm_tasks_count.load(Ordering::Relaxed)
                            == cloned_cr_ref3.saved_key_counter
                        {
                            log("finishing and stopping parallel transactions thread..."
                                .to_string());
                            {
                                let mut tpool = cloned_cr_ref3.thread_pool.lock().unwrap();
                                tpool.stop_pool();
                            }
                            cloned_cr_ref3.collect_results();
                            return;
                        }
                        let mut cloned_outputs: Option<HashMap<i32, Arc<Mutex<WasmTask>>>> = None;
                        {
                            log("ok 20".to_string());

                            let cloned_task1 = Arc::clone(&task);
                            let cloned_task_ref1 = cloned_task1.lock().unwrap();

                            log("ok 21".to_string());

                            let cloned_outputs_1 = Arc::clone(&cloned_task_ref1.outputs);
                            let cloned_outputs_ref1 = cloned_outputs_1.lock().unwrap();

                            log("ok 22".to_string());

                            cloned_outputs = Some(cloned_outputs_ref1.clone());
                        }

                        log("ok 23".to_string());

                        for (_, val) in cloned_outputs.unwrap().iter() {
                            log("ok 24".to_string());

                            let cloned_task_t2 = Arc::clone(&val);
                            let cloned_task_ref_t2 = cloned_task_t2.lock().unwrap();

                            log("ok 25".to_string());

                            if !cloned_task_ref_t2.started {
                                log("ok 26".to_string());

                                let cloned_task1 = Arc::clone(&task);
                                let cloned_task_ref1 = cloned_task1.lock().unwrap();

                                log("ok 27".to_string());

                                cloned_task_ref_t2
                                    .inputs
                                    .lock()
                                    .unwrap()
                                    .get_mut(&cloned_task_ref1.id.clone())
                                    .unwrap()
                                    .0 = true;

                                log("ok 28".to_string());

                                let cloned_task_t3 = Arc::clone(&val);
                                next_wasm_tasks.push(cloned_task_t3);

                                log("ok 29".to_string());
                            }
                        }
                    }

                    log("ok 30".to_string());

                    for t in next_wasm_tasks {
                        log("ok 31".to_string());

                        let cloned_t = Arc::clone(&t);
                        let cloned_cr4 = Arc::clone(&GLOBAL_CR);
                        let mut cloned_cr_ref4 = cloned_cr4.lock().unwrap();

                        log("ok 32".to_string());

                        cloned_cr_ref4.exec_wasm_task(cloned_t);
                    }
                });
            }
        }
    }
}

impl Clone for ChainTrx {
    fn clone(&self) -> Self {
        ChainTrx {
            machine_id: self.machine_id.clone(),
            callback_id: self.callback_id.clone(),
            input: self.input.clone(),
            user_id: self.user_id.clone(),
            key: self.key.clone(),
        }
    }
}
