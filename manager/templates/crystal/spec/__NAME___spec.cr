require "./spec_helper"

describe "raylib-cr" do
  it "links raylib" do
    Raylib::RAYWHITE.r.should eq(245)
  end
end
