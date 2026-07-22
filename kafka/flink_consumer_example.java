/**
 * [修复] Kafka Flink 消费测试示例代码
 * 用途: 消费秒杀系统的用户行为埋点数据，进行实时分析
 * 技术栈: Apache Flink 1.18 + Kafka Connector
 * 运行方式: 在 Flink 集群中提交此 Job，或本地 IDE 运行测试
 *
 * 数据流: Kafka(topic: miaosha-behavior-track) → Flink → 实时指标计算
 * 消费字段: trace_id, user_id, product_id, action, request_ip, result, cost_ms, fail_reason, instance_id, timestamp
 */

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.functions.FlatMapFunction;
import org.apache.flink.api.common.serialization.SimpleStringSchema;
import org.apache.flink.api.java.tuple.Tuple2;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.windowing.assigners.TumblingProcessingTimeWindows;
import org.apache.flink.streaming.api.windowing.time.Time;
import org.apache.flink.util.Collector;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

public class SeckillBehaviorAnalysis {

    public static void main(String[] args) throws Exception {
        // 1. 创建 Flink 流处理执行环境
        final StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(2); // 设置并行度

        // 2. 配置 Kafka Source（消费秒杀行为埋点数据）
        KafkaSource<String> kafkaSource = KafkaSource.<String>builder()
            .setBootstrapServers("127.0.0.1:9092")        // Kafka broker 地址
            .setTopics("miaosha-behavior-track")           // 消费的 topic
            .setGroupId("flink-seckill-consumer-group")    // 消费者组 ID
            .setStartingOffsets(OffsetsInitializer.latest()) // 从最新 offset 开始消费
            .setValueOnlyDeserializer(new SimpleStringSchema())
            .build();

        // 3. 从 Kafka 读取数据流
        DataStream<String> kafkaStream = env
            .fromSource(kafkaSource, WatermarkStrategy.noWatermarks(), "Kafka Source");

        // 4. JSON 解析器
        ObjectMapper mapper = new ObjectMapper();

        // 5. 实时统计：每秒各 action 类型的请求量
        DataStream<Tuple2<String, Integer>> actionCount = kafkaStream
            .flatMap(new FlatMapFunction<String, Tuple2<String, Integer>>() {
                @Override
                public void flatMap(String value, Collector<Tuple2<String, Integer>> out) throws Exception {
                    try {
                        JsonNode node = mapper.readTree(value);
                        String action = node.has("action") ? node.get("action").asText() : "unknown";
                        out.collect(new Tuple2<>(action, 1));
                    } catch (Exception e) {
                        // 解析失败的消息跳过
                        System.err.println("JSON解析失败: " + e.getMessage());
                    }
                }
            })
            .keyBy(tuple -> tuple.f0)  // 按 action 类型分组
            .window(TumblingProcessingTimeWindows.of(Time.seconds(10))) // 10秒滚动窗口
            .sum(1); // 聚合计数

        // 6. 打印结果到控制台（生产环境可写入 MySQL/Redis/ClickHouse 等）
        actionCount.print("行为统计");

        // 7. 实时统计：每秒秒杀成功/失败数量
        DataStream<Tuple2<String, Integer>> seckillResult = kafkaStream
            .flatMap(new FlatMapFunction<String, Tuple2<String, Integer>>() {
                @Override
                public void flatMap(String value, Collector<Tuple2<String, Integer>> out) throws Exception {
                    try {
                        JsonNode node = mapper.readTree(value);
                        String action = node.has("action") ? node.get("action").asText() : "";
                        // 只关注秒杀相关行为
                        if ("success".equals(action)) {
                            out.collect(new Tuple2<>("秒杀成功", 1));
                        } else if ("fail".equals(action)) {
                            // 提取失败原因
                            String reason = node.has("fail_reason") ? node.get("fail_reason").asText() : "未知";
                            out.collect(new Tuple2<>("秒杀失败-" + reason, 1));
                        }
                    } catch (Exception e) {
                        // 跳过解析失败的消息
                    }
                }
            })
            .keyBy(tuple -> tuple.f0)
            .window(TumblingProcessingTimeWindows.of(Time.seconds(10)))
            .sum(1);

        seckillResult.print("秒杀结果统计");

        // 8. 实时统计：各商品秒杀热度 TOP-N
        DataStream<Tuple2<String, Integer>> productHeat = kafkaStream
            .flatMap(new FlatMapFunction<String, Tuple2<String, Integer>>() {
                @Override
                public void flatMap(String value, Collector<Tuple2<String, Integer>> out) throws Exception {
                    try {
                        JsonNode node = mapper.readTree(value);
                        if (node.has("product_id")) {
                            String productId = "商品" + node.get("product_id").asText();
                            out.collect(new Tuple2<>(productId, 1));
                        }
                    } catch (Exception e) {
                        // 跳过解析失败
                    }
                }
            })
            .keyBy(tuple -> tuple.f0)
            .window(TumblingProcessingTimeWindows.of(Time.seconds(30)))
            .sum(1);

        productHeat.print("商品热度");

        // 9. 启动 Flink Job
        env.execute("秒杀行为实时分析 - Flink Job");
    }
}

/**
 * 部署说明:
 * 1. 确保 Kafka 集群已启动，topic miaosha-behavior-track 已创建
 * 2. 将此文件编译打包为 JAR: mvn clean package
 * 3. 提交到 Flink 集群: flink run -c SeckillBehaviorAnalysis target/seckill-flink-1.0.jar
 * 4. 或直接在 IDE 中运行 main 方法测试
 *
 * 依赖配置 (pom.xml):
 * <dependencies>
 *   <dependency>
 *     <groupId>org.apache.flink</groupId>
 *     <artifactId>flink-streaming-java</artifactId>
 *     <version>1.18.0</version>
 *   </dependency>
 *   <dependency>
 *     <groupId>org.apache.flink</groupId>
 *     <artifactId>flink-connector-kafka</artifactId>
 *     <version>1.18.0</version>
 *   </dependency>
 *   <dependency>
 *     <groupId>com.fasterxml.jackson.core</groupId>
 *     <artifactId>jackson-databind</artifactId>
 *     <version>2.15.2</version>
 *   </dependency>
 * </dependencies>
 */