from diagrams import Cluster, Diagram, Edge
from diagrams.k8s.compute import Deployment, Pod
from diagrams.k8s.controlplane import API
from diagrams.k8s.group import NS
from diagrams.k8s.podconfig import Secret
from diagrams.oci.compute import OCIR

with Diagram("regcred-injector", show=True):

    api = API("Control Plane")
    ocir = OCIR("DockerHub")

    injector = None

    with Cluster("kube-system"):
      injector = Deployment("regcred-injector")
      secret = Secret("Credential/Certs")

      api << Edge(label="1 mutate webhook") << injector << Edge(label="4 return mutated response") << api
      injector >> Edge(label="2 fetch credential") >> secret


    with Cluster("default"):
      pod = Pod("new-pod")
      secret = Secret("regcred")
      api >> Edge(label="5 create pod") >> pod >> Edge(label="6 use registry credential") >> secret
      injector >> Edge(label="3 create registry credential") >> secret
      pod >> Edge(label="7 authenticated pull") >> ocir